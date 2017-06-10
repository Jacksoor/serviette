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
	"syscall"

	"github.com/bwmarrin/discordgo"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type outputFormatter func(r *scriptspb.ExecuteResponse) (*discordgo.MessageSend, error)

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

var outputFormatters map[string]outputFormatter = map[string]outputFormatter{
	"text": func(r *scriptspb.ExecuteResponse) (*discordgo.MessageSend, error) {
		embed := &discordgo.MessageEmbed{
			Fields: []*discordgo.MessageEmbedField{},
		}

		if syscall.WaitStatus(r.WaitStatus).ExitStatus() != 0 {
			embed.Color = 0xb50000
		} else {
			embed.Color = 0x009100
		}

		stdout := r.Stdout
		if len(stdout) > 1500 {
			stdout = stdout[:1500]
		}

		embed.Description = string(stdout)

		return &discordgo.MessageSend{Embed: embed}, nil
	},

	"discord.embed": func(r *scriptspb.ExecuteResponse) (*discordgo.MessageSend, error) {
		embed := &discordgo.MessageEmbed{}

		if err := json.Unmarshal(r.Stdout, embed); err != nil {
			return nil, invalidOutputError{err}
		}

		return &discordgo.MessageSend{
			Embed: embed,
		}, nil
	},

	"discord.embed_multipart": func(r *scriptspb.ExecuteResponse) (*discordgo.MessageSend, error) {
		embed := &discordgo.MessageEmbed{}

		msg, err := mail.ReadMessage(bytes.NewReader(r.Stdout))
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

		return &discordgo.MessageSend{
			Embed: embed,
			Files: files,
		}, nil
	},
}
