package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type OutputFormatter func(authorID string, content []byte, ok bool) (*discordgo.MessageSend, error)

type invalidOutputError struct {
	error
}

func copyPart(dest io.Writer, part *multipart.Part) (int64, error) {
	encoding := part.Header.Get("Content-Transfer-Encoding")

	switch encoding {
	case "base64":
		dec := base64.NewDecoder(base64.StdEncoding, part)
		return io.Copy(dest, dec)
	case "":
		return io.Copy(dest, part)
	}

	return 0, fmt.Errorf("unknown Content-Transfer-Encoding: %s", encoding)
}

var outputSigils = map[bool]string{
	true:  "✅",
	false: "❎",
}

type RichContentField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type RichContent struct {
	Fallback      string              `json:"fallback"`
	Color         int                 `json:"color"`
	Author        string              `json:"author"`
	AuthorLink    string              `json:"authorLink"`
	AuthorIconURL string              `json:"authorIconURL"`
	Title         string              `json:"title"`
	TitleLink     string              `json:"titleLink"`
	Text          string              `json:"text"`
	Fields        []*RichContentField `json:"fields"`
	ImageURL      string              `json:"imageURL"`
	ThumbnailURL  string              `json:"thumbnailURL"`
	Footer        string              `json:"footer"`
	FooterIconURL string              `json:"footerIconURL"`
	Timestamp     *int64              `json:"timestamp"`
}

var OutputFormatters map[string]OutputFormatter = map[string]OutputFormatter{
	"raw": func(authorID string, content []byte, ok bool) (*discordgo.MessageSend, error) {
		return &discordgo.MessageSend{Content: string(content)}, nil
	},

	"text": func(authorID string, content []byte, ok bool) (*discordgo.MessageSend, error) {
		embed := &discordgo.MessageEmbed{
			Fields: []*discordgo.MessageEmbedField{},
		}

		if ok {
			embed.Color = 0x009100
		} else {
			embed.Color = 0xb50000
		}

		if len(content) > 1500 {
			content = content[:1500]
		}

		embed.Description = string(content)

		note := outputSigils[ok]
		if authorID != "" {
			note = fmt.Sprintf("<@%s>: %s", authorID, outputSigils[ok])
		}

		return &discordgo.MessageSend{
			Content: note,
			Embed:   embed,
		}, nil
	},

	"rich": func(authorID string, content []byte, ok bool) (*discordgo.MessageSend, error) {
		richContent := &RichContent{}

		if err := json.Unmarshal(content, richContent); err != nil {
			return nil, invalidOutputError{err}
		}

		note := outputSigils[ok]
		if authorID != "" {
			note = fmt.Sprintf("<@%s>: %s", authorID, outputSigils[ok])
		}

		fields := make([]*discordgo.MessageEmbedField, len(richContent.Fields))
		for i, richField := range richContent.Fields {
			fields[i] = &discordgo.MessageEmbedField{
				Name:   richField.Name,
				Value:  richField.Value,
				Inline: richField.Inline,
			}
		}

		var ts string
		if richContent.Timestamp != nil {
			ts = time.Unix(*richContent.Timestamp, 0).UTC().Format(time.RFC3339)
		}

		embed := &discordgo.MessageEmbed{
			URL:         richContent.TitleLink,
			Type:        "rich",
			Title:       richContent.Title,
			Description: richContent.Text,
			Timestamp:   ts,
			Fields:      fields,
			Color:       richContent.Color,
		}

		if richContent.ImageURL != "" {
			embed.Image = &discordgo.MessageEmbedImage{
				URL: richContent.ImageURL,
			}
		}

		if richContent.Author != "" {
			embed.Author = &discordgo.MessageEmbedAuthor{
				Name:    richContent.Author,
				URL:     richContent.AuthorLink,
				IconURL: richContent.AuthorIconURL,
			}
		}

		if richContent.ThumbnailURL != "" {
			embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
				URL: richContent.ThumbnailURL,
			}
		}

		if richContent.Footer != "" {
			embed.Footer = &discordgo.MessageEmbedFooter{
				Text:    richContent.Footer,
				IconURL: richContent.FooterIconURL,
			}
		}

		return &discordgo.MessageSend{
			Content: note,
			Embed:   embed,
		}, nil
	},

	"discord.embed": func(authorID string, content []byte, ok bool) (*discordgo.MessageSend, error) {
		embed := &discordgo.MessageEmbed{}

		if err := json.Unmarshal(content, embed); err != nil {
			return nil, invalidOutputError{err}
		}

		note := outputSigils[ok]
		if authorID != "" {
			note = fmt.Sprintf("<@%s>: %s", authorID, outputSigils[ok])
		}

		return &discordgo.MessageSend{
			Content: note,
			Embed:   embed,
		}, nil
	},

	"discord.embed_multipart": func(authorID string, content []byte, ok bool) (*discordgo.MessageSend, error) {
		embed := &discordgo.MessageEmbed{}

		msg, err := mail.ReadMessage(bytes.NewReader(content))
		if err != nil {
			return nil, invalidOutputError{err}
		}

		mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
		if err != nil {
			return nil, invalidOutputError{err}
		}

		if !strings.HasPrefix(mediaType, "multipart/") {
			return nil, invalidOutputError{err}
		}

		boundary, ok := params["boundary"]
		if !ok {
			return nil, invalidOutputError{errors.New("boundary not found in multipart header")}
		}

		mr := multipart.NewReader(msg.Body, boundary)

		// Parse the payload (first part).
		payloadPart, err := mr.NextPart()
		if err != nil {
			return nil, invalidOutputError{err}
		}

		// Decode the embed.
		var payloadBuf bytes.Buffer
		if _, err := copyPart(&payloadBuf, payloadPart); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(payloadBuf.Bytes(), embed); err != nil {
			return nil, invalidOutputError{err}
		}

		// Decode all the files.
		files := make([]*discordgo.File, 0)
		for {
			filePart, err := mr.NextPart()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, invalidOutputError{err}
			}

			buf := new(bytes.Buffer)
			if _, err := copyPart(buf, filePart); err != nil {
				return nil, err
			}

			files = append(files, &discordgo.File{
				Name:        filePart.FileName(),
				ContentType: filePart.Header.Get("Content-Type"),
				Reader:      buf,
			})
		}

		note := outputSigils[ok]
		if authorID != "" {
			note = fmt.Sprintf("<@%s>: %s", authorID, outputSigils[ok])
		}

		return &discordgo.MessageSend{
			Content: note,
			Embed:   embed,
			Files:   files,
		}, nil
	},
}
