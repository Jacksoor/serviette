// The rpc package is a ripoff of net/rpc/jsonrpc, but instead of using a json.Decoder it reads from the socket one byte at a time in order to be able to read ancillary data.
//
// The license for net/rpc/jsonrpc is available at https://golang.org/LICENSE

package rpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/rpc"
	"os"
	"reflect"
	"sync"

	"golang.org/x/sys/unix"
)

var (
	ErrInvalidRequest  error = errors.New("invalid request")
	ErrInvalidResponse       = errors.New("invalid response")
	ErrInvalidConn           = errors.New("invalid connection passed")
)

type Response struct {
	Body  interface{}
	Files []*os.File
}

type serverCodec struct {
	file *os.File
	enc  *json.Encoder
	buf  bytes.Buffer

	req serverRequest

	mutex   sync.Mutex
	seq     uint64
	pending map[uint64]*json.RawMessage
}

type serverRequest struct {
	Id     *json.RawMessage `json:"id"`
	Method string           `json:"method"`
	Params *json.RawMessage `json:"params"`
}

func (r *serverRequest) reset() {
	r.Method = ""
	r.Params = nil
	r.Id = nil
}

type serverResponse struct {
	Id     *json.RawMessage `json:"id"`
	Result interface{}      `json:"result"`
	Error  interface{}      `json:"error"`
}

func (c *serverCodec) ReadRequestHeader(r *rpc.Request) error {
	c.req.reset()

	buf := make([]byte, 1)
	for {
		if _, err := c.file.Read(buf); err != nil {
			return err
		}
		c.buf.Write(buf)
		if buf[0] == '\n' {
			break
		}
	}

	if err := json.Unmarshal(c.buf.Bytes(), &c.req); err != nil {
		return err
	}
	c.buf.Reset()

	r.ServiceMethod = c.req.Method
	c.mutex.Lock()
	c.seq++
	c.pending[c.seq] = c.req.Id
	c.req.Id = nil
	r.Seq = c.seq

	c.mutex.Unlock()

	return nil
}

func (c *serverCodec) ReadRequestBody(x interface{}) error {
	if x == nil {
		return nil
	}

	if c.req.Params == nil {
		return ErrInvalidRequest
	}

	var params [1]interface{}
	params[0] = x

	if err := json.Unmarshal(*c.req.Params, &params); err != nil {
		return err
	}

	rval := reflect.ValueOf(x).Elem()
	unixRightsField := rval.FieldByName("UnixRights")

	if unixRightsField.IsValid() && unixRightsField.Type().AssignableTo(reflect.TypeOf(([]*os.File)(nil))) {
		// Read the oob FDs.
		dummy := make([]byte, 1)
		oob := make([]byte, 4096)

		n, oobn, _, _, err := unix.Recvmsg(int(c.file.Fd()), dummy, oob, 0)
		if err != nil {
			return err
		}

		if n != len(dummy) || oobn <= 0 {
			return fmt.Errorf("incorrect number of bytes read: n = %d (%v), oobn = %d (%v)", n, dummy, oobn, oob)
		}

		scms, err := unix.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			return err
		}

		if len(scms) != 1 {
			return fmt.Errorf("received more than 1 socket control message: len(scms) = %d", len(scms))
		}

		fds, err := unix.ParseUnixRights(&scms[0])
		if err != nil {
			return err
		}

		files := make([]*os.File, len(fds))
		for i, fd := range fds {
			files[i] = os.NewFile(uintptr(fd), fmt.Sprintf("received fd %d", fd))
		}

		unixRightsField.Set(reflect.ValueOf(files))
	}

	return nil
}

var null = json.RawMessage([]byte("null"))

func (c *serverCodec) writeBody(r *rpc.Response, x interface{}) error {
	c.mutex.Lock()
	b, ok := c.pending[r.Seq]

	if !ok {
		c.mutex.Unlock()
		return ErrInvalidResponse
	}
	delete(c.pending, r.Seq)
	c.mutex.Unlock()

	if b == nil {
		b = &null
	}

	resp := serverResponse{Id: b}
	if r.Error == "" {
		resp.Result = x
	} else {
		resp.Error = r.Error
	}
	return c.enc.Encode(resp)
}

func (c *serverCodec) WriteResponse(r *rpc.Response, x interface{}) error {
	if r.Error != "" {
		return c.writeBody(r, nil)
	}

	payload, ok := x.(*Response)
	if !ok {
		return ErrInvalidResponse
	}

	if err := c.writeBody(r, payload.Body); err != nil {
		return err
	}

	if payload.Files != nil {
		fds := make([]int, len(payload.Files))
		for i, file := range payload.Files {
			fds[i] = int(file.Fd())
		}

		if err := unix.Sendmsg(int(c.file.Fd()), []byte{0}, unix.UnixRights(fds...), nil, 0); err != nil {
			return err
		}

		for _, file := range payload.Files {
			file.Close()
		}
	}

	return nil
}

func (c *serverCodec) Close() error {
	return c.file.Close()
}

func NewServerCodec(conn io.ReadWriteCloser) (rpc.ServerCodec, error) {
	file, ok := conn.(*os.File)
	if !ok {
		return nil, ErrInvalidConn
	}

	return &serverCodec{
		file: file,
		enc:  json.NewEncoder(file),

		pending: make(map[uint64]*json.RawMessage, 0),
	}, nil
}
