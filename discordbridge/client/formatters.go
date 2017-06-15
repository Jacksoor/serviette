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

var sigils = map[bool]string{
	true:  "✅",
	false: "❎",
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

		note := sigils[ok]
		if authorID != "" {
			note = fmt.Sprintf("<@%s>: %s", authorID, sigils[ok])
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

		note := sigils[ok]
		if authorID != "" {
			note = fmt.Sprintf("<@%s>: %s", authorID, sigils[ok])
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

		note := sigils[ok]
		if authorID != "" {
			note = fmt.Sprintf("<@%s>: %s", authorID, sigils[ok])
		}

		return &discordgo.MessageSend{
			Content: note,
			Embed:   embed,
			Files:   files,
		}, nil
	},
}
