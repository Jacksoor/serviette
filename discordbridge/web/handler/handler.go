package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang/glog"

	"github.com/bwmarrin/discordgo"
	"github.com/julienschmidt/httprouter"
	"github.com/lib/pq"
	"golang.org/x/oauth2"
)

type Handler struct {
	http.Handler

	session *discordgo.Session

	defaultAnnouncement string
	oauthConf           *oauth2.Config
	db                  *sql.DB
}

func New(baseURL string, clientID string, clientSecret string, botToken string, defaultAnnouncement string, db *sql.DB) (*Handler, error) {
	session, err := discordgo.New(fmt.Sprintf("Bot %s", botToken))
	if err != nil {
		return nil, err
	}
	session.StateEnabled = false

	router := httprouter.New()

	h := &Handler{
		Handler: router,

		session: session,

		defaultAnnouncement: defaultAnnouncement,
		oauthConf: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       []string{"bot"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://discordapp.com/api/oauth2/authorize",
				TokenURL: "https://discordapp.com/api/oauth2/token",
			},
			RedirectURL: fmt.Sprintf("%s/invites/bind", baseURL),
		},
		db: db,
	}

	router.GET("/invites/use", h.inviteUse)
	router.GET("/invites/bind", h.inviteBind)
	router.GET("/invites/roles", h.inviteRoles)
	router.POST("/invites/consume", h.inviteConsume)

	return h, nil
}

func (h *Handler) inviteUse(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	query := r.URL.Query()

	inviteID := query.Get("invite_id")

	var boundGuildID *string
	if err := h.db.QueryRowContext(r.Context(), `
		select bound_guild_id
		from invites
		where invite_id = $1
		for update
	`, inviteID).Scan(&boundGuildID); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		glog.Errorf("Failed to find invite: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var guildID string
	if boundGuildID != nil {
		guildID = *boundGuildID
	}

	http.Redirect(w, r, h.oauthConf.AuthCodeURL(inviteID, oauth2.AccessTypeOnline, oauth2.SetAuthURLParam("guild_id", guildID), oauth2.SetAuthURLParam("permissions", "0")), http.StatusFound)
}

func (h *Handler) inviteBind(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	query := r.URL.Query()

	inviteID := query.Get("state")
	guildID := query.Get("guild_id")

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Don't allow binding the invite to a guild if the guild already has an admin role.
	var adminRoleID string
	if err := tx.QueryRowContext(r.Context(), `
		select admin_role_id
		from guild_vars
		where guild_id = $1
	`, guildID).Scan(&adminRoleID); err != nil {
		if err != sql.ErrNoRows {
			glog.Errorf("Failed to check guild vars: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	if adminRoleID != "" {
		http.Error(w, "Forbidden (server already set up)", http.StatusForbidden)
		return
	}

	var boundGuildID *string
	if err := tx.QueryRowContext(r.Context(), `
		select bound_guild_id
		from invites
		where invite_id = $1
		for update
	`, inviteID).Scan(&boundGuildID); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		glog.Errorf("Failed to find invite: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if boundGuildID != nil && *boundGuildID != guildID {
		http.Error(w, "Forbidden (invite already used for a different server)", http.StatusForbidden)
		return
	}

	if _, err := tx.ExecContext(r.Context(), `
		insert into guild_vars (guild_id, admin_role_id, announcement)
		values ($1, '', $2)
		on conflict do nothing
	`, guildID, h.defaultAnnouncement); err != nil {
		glog.Errorf("Failed to add guild var: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := tx.ExecContext(r.Context(), `
		update invites
		set bound_guild_id = $1
		where invite_id = $2
	`, guildID, inviteID); err != nil {
		if err, ok := err.(*pq.Error); ok {
			if err.Code == "23505" /* unique_violation */ {
				http.Error(w, "Forbidden (invite already being used)", http.StatusForbidden)
				return
			}
		}
		glog.Errorf("Failed to bind invite: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit transaction: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := h.oauthConf.Exchange(r.Context(), query.Get("code")); err != nil {
		glog.Errorf("Failed to exchange token: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!doctype html>
<html>
<head>
<title>Authorization Success</title>
<meta charset="utf-8">
</head>
<body>
This window will close automatically.
<script>
window.opener.onKobunInviteBound();
window.close();
</script>
</body>
</html>`))
}

func (h *Handler) inviteRoles(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	query := r.URL.Query()

	inviteID := query.Get("invite_id")

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var guildID string
	if err := tx.QueryRowContext(r.Context(), `
		select bound_guild_id from invites
		where invite_id = $1
	`, inviteID).Scan(&guildID); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		glog.Errorf("Failed to find invite: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	guild, err := h.session.Guild(guildID)
	if err != nil {
		glog.Errorf("Failed to find guild: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	roleNames := make(map[string]string, len(guild.Roles))
	for _, role := range guild.Roles {
		roleNames[role.ID] = role.Name
	}

	w.Header().Set("Content-Type", "application/json")
	raw, err := json.Marshal(struct {
		Roles map[string]string `json:"roles"`
	}{
		roleNames,
	})
	if err != nil {
		glog.Errorf("Failed to marshal JSON: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Write(raw)
}

func (h *Handler) inviteConsume(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if err := r.ParseForm(); err != nil {
		glog.Errorf("Failed to parse form: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	inviteID := r.Form.Get("invite_id")

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var guildID string
	if err := tx.QueryRowContext(r.Context(), `
		select bound_guild_id
		from invites
		where invite_id = $1
		for update
	`, inviteID).Scan(&guildID); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		glog.Errorf("Failed to find invite: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := tx.ExecContext(r.Context(), `
		update guild_vars
		set admin_role_id = $1
		where guild_id = $2
	`, r.Form.Get("role_id"), guildID); err != nil {
		glog.Errorf("Failed to add guild var: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := tx.ExecContext(r.Context(), `
		delete from invites
		where invite_id = $1
	`, inviteID); err != nil {
		glog.Errorf("Failed to delete invite: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit transaction: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
