package rest

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/emicklei/go-restful"
	"github.com/golang/glog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/restbridge/auth"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Script struct {
	OwnerName   string `json:"ownerName"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Visibility  int    `json:"visibility"`
	Content     string `json:"content,omitempty"`
}

type ScriptsResource struct {
	authenticator *auth.Authenticator
	scriptsClient scriptspb.ScriptsClient
}

func NewScriptsResource(authenticator *auth.Authenticator, scriptsClient scriptspb.ScriptsClient) *ScriptsResource {
	return &ScriptsResource{
		authenticator: authenticator,
		scriptsClient: scriptsClient,
	}
}

func (r ScriptsResource) WebService() *restful.WebService {
	ws := new(restful.WebService)

	ws.Path("/scripts").
		Doc("Scripts information.").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	ws.Route(ws.GET("").To(r.list).
		Doc("List scripts.").
		Writes([]*Script{}))

	ws.Route(ws.GET("/{accountName}").To(r.listAccount).
		Doc("Lists account scripts.").
		Param(ws.PathParameter("accountName", "account name")).
		Writes([]*Script{}))

	ws.Route(ws.GET("/{accountName}/{scriptName}").To(r.read).
		Doc("Reads a script.").
		Param(ws.PathParameter("accountName", "account name")).
		Param(ws.PathParameter("scriptName", "script name")).
		Writes(Script{}))

	ws.Route(ws.POST("").To(r.create).
		Doc("Creates a script.").
		Reads(Script{}))

	ws.Route(ws.PUT("/{accountName}/{scriptName}").To(r.update).
		Doc("Updates a script.").
		Param(ws.PathParameter("accountName", "account name")).
		Param(ws.PathParameter("scriptName", "script name")).
		Reads(Script{}))

	ws.Route(ws.DELETE("/{accountName}/{scriptName}").To(r.delete).
		Doc("Deletes a script.").
		Param(ws.PathParameter("accountName", "account name")).
		Param(ws.PathParameter("scriptName", "script name")).
		Reads(Script{}))

	return ws
}

func (r ScriptsResource) list(req *restful.Request, resp *restful.Response) {
	username, err := r.authenticator.Authenticate(req, resp)
	if err != nil {
		glog.Errorf("Failed to authenticate: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	var offset uint32
	limit := maxLimit

	if rawOffset := req.QueryParameter("offset"); rawOffset != "" {
		v, err := strconv.ParseUint(rawOffset, 10, 32)
		if err != nil {
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "bad request: bad offset")
		}

		offset = uint32(v)
	}

	if rawLimit := req.QueryParameter("limit"); rawLimit != "" {
		v, err := strconv.ParseUint(rawLimit, 10, 32)
		if err != nil {
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "bad request: bad limit")
		}

		limit = uint32(v)

		if limit > maxLimit {
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "bad request: limit too high")
		}
	}

	listResp, err := r.scriptsClient.List(req.Request.Context(), &scriptspb.ListRequest{
		OwnerName:  "",
		Query:      req.QueryParameter("q"),
		ViewerName: username,
		Offset:     offset,
		Limit:      limit,
	})

	if err != nil {
		glog.Errorf("Failed to list scripts: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	scripts := make([]*Script, len(listResp.Entry))
	for i, entry := range listResp.Entry {
		scripts[i] = &Script{
			OwnerName:   entry.OwnerName,
			Name:        entry.Name,
			Description: entry.Meta.Description,
			Visibility:  int(entry.Meta.Visibility),
		}
	}

	resp.WriteEntity(scripts)
}

func (r ScriptsResource) listAccount(req *restful.Request, resp *restful.Response) {
	username, err := r.authenticator.Authenticate(req, resp)
	if err != nil {
		glog.Errorf("Failed to authenticate: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	accountName := req.PathParameter("accountName")

	var offset uint32
	limit := maxLimit

	if rawOffset := req.QueryParameter("offset"); rawOffset != "" {
		v, err := strconv.ParseUint(rawOffset, 10, 32)
		if err != nil {
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "bad request: bad offset")
		}

		offset = uint32(v)
	}

	if rawLimit := req.QueryParameter("limit"); rawLimit != "" {
		v, err := strconv.ParseUint(rawLimit, 10, 32)
		if err != nil {
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "bad request: bad limit")
		}

		limit = uint32(v)

		if limit > maxLimit {
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "bad request: limit too high")
		}
	}

	listResp, err := r.scriptsClient.List(req.Request.Context(), &scriptspb.ListRequest{
		OwnerName:  accountName,
		Query:      req.QueryParameter("q"),
		ViewerName: username,
		Offset:     offset,
		Limit:      limit,
	})

	if err != nil {
		glog.Errorf("Failed to list scripts: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	scripts := make([]*Script, len(listResp.Entry))
	for i, entry := range listResp.Entry {
		scripts[i] = &Script{
			OwnerName:   entry.OwnerName,
			Name:        entry.Name,
			Description: entry.Meta.Description,
			Visibility:  int(entry.Meta.Visibility),
		}
	}

	resp.WriteEntity(scripts)
}

var errNotPublished = errors.New("not published")

func (r ScriptsResource) read(req *restful.Request, resp *restful.Response) {
	username, err := r.authenticator.Authenticate(req, resp)
	if err != nil {
		glog.Errorf("Failed to authenticate: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	accountName := req.PathParameter("accountName")
	scriptName := req.PathParameter("scriptName")

	var meta *scriptspb.Meta
	var content string

	var g errgroup.Group

	g.Go(func() error {
		metaResp, err := r.scriptsClient.GetMeta(req.Request.Context(), &scriptspb.GetMetaRequest{
			OwnerName: accountName,
			Name:      scriptName,
		})
		if err != nil {
			return err
		}

		meta = metaResp.Meta
		if meta.Visibility == scriptspb.Visibility_UNPUBLISHED && username != accountName {
			return errNotPublished
		}
		return nil
	})

	if req.QueryParameter("getContent") != "" {
		g.Go(func() error {
			contentResp, err := r.scriptsClient.GetContent(req.Request.Context(), &scriptspb.GetContentRequest{
				OwnerName: accountName,
				Name:      scriptName,
			})
			if err != nil {
				return err
			}

			content = string(contentResp.Content)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		if err == errNotPublished {
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusNotFound, "script not found")
			return
		}

		switch grpc.Code(err) {
		case codes.InvalidArgument:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "script name invalid")
		case codes.NotFound:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusNotFound, "script not found")
		default:
			glog.Errorf("Failed to get script: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		}

		return
	}

	resp.WriteEntity(Script{
		OwnerName:   accountName,
		Name:        scriptName,
		Description: meta.Description,
		Visibility:  int(meta.Visibility),
		Content:     content,
	})
}

func (r ScriptsResource) create(req *restful.Request, resp *restful.Response) {
	username, err := r.authenticator.Authenticate(req, resp)
	if err != nil {
		glog.Errorf("Failed to authenticate: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	script := new(Script)
	if err := req.ReadEntity(&script); err != nil {
		glog.Errorf("Failed to read entity: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	if script.OwnerName != username {
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusUnauthorized, "unauthorized")
		return
	}

	if _, err := r.scriptsClient.Create(req.Request.Context(), &scriptspb.CreateRequest{
		OwnerName: script.OwnerName,
		Name:      script.Name,
		Meta: &scriptspb.Meta{
			Description: script.Description,
			Visibility:  scriptspb.Visibility(script.Visibility),
		},
		Content: []byte(strings.Replace(script.Content, "\r", "", -1)),
	}); err != nil {
		switch grpc.Code(err) {
		case codes.InvalidArgument:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "script name invalid")
		case codes.AlreadyExists:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusConflict, "script already exists")
		default:
			glog.Errorf("Failed to create script: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		}
		return
	}

	resp.WriteEntity(script)
}

func (r ScriptsResource) update(req *restful.Request, resp *restful.Response) {
	username, err := r.authenticator.Authenticate(req, resp)
	if err != nil {
		glog.Errorf("Failed to authenticate: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	accountName := req.PathParameter("accountName")
	scriptName := req.PathParameter("scriptName")

	script := new(Script)
	if err := req.ReadEntity(&script); err != nil {
		glog.Errorf("Failed to read entity: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	if script.OwnerName != username || script.OwnerName != accountName {
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusUnauthorized, "unauthorized")
		return
	}

	if _, err := r.scriptsClient.Delete(req.Request.Context(), &scriptspb.DeleteRequest{
		OwnerName: script.OwnerName,
		Name:      scriptName,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.InvalidArgument:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "script name invalid")
		case codes.NotFound:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusNotFound, "script not found")
		default:
			glog.Errorf("Failed to delete script: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		}
		return
	}

	if _, err := r.scriptsClient.Create(req.Request.Context(), &scriptspb.CreateRequest{
		OwnerName: script.OwnerName,
		Name:      script.Name,
		Meta: &scriptspb.Meta{
			Description: script.Description,
			Visibility:  scriptspb.Visibility(script.Visibility),
		},
		Content: []byte(strings.Replace(script.Content, "\r", "", -1)),
	}); err != nil {
		glog.Errorf("Failed to create script: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	resp.WriteEntity(script)
}

func (r ScriptsResource) delete(req *restful.Request, resp *restful.Response) {
	username, err := r.authenticator.Authenticate(req, resp)
	if err != nil {
		glog.Errorf("Failed to authenticate: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	accountName := req.PathParameter("accountName")

	if username != accountName {
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusUnauthorized, "unauthorized")
		return
	}

	scriptName := req.PathParameter("scriptName")

	if _, err := r.scriptsClient.Delete(req.Request.Context(), &scriptspb.DeleteRequest{
		OwnerName: accountName,
		Name:      scriptName,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.InvalidArgument:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "script name invalid")
		case codes.NotFound:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusNotFound, "script not found")
		default:
			glog.Errorf("Failed to delete script: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		}
		return
	}
}
