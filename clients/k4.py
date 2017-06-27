import array
import functools
import json
import os
import socket
import sys


class Expando(object):
    def __init__(self, **kwargs):
        for k, v in kwargs.items():
            setattr(self, k, v)


def load_expando(s):
    return json.loads(s, object_hook=lambda d: Expando(**d))


class RPCError(Exception):
    pass


class ClientError(RPCError):
    pass


class MismatchedIDError(ClientError):
    pass


class ServerError(RPCError):
    pass


class _Stub(object):
    def __init__(self, client, name):
        self._client = client
        self._name = name

    def __getattr__(self, name):
        return functools.partial(self._client.call, "{}.{}".format(
            self._name, name))


class Child(object):
    def __init__(self, client, handle):
        self._client = client
        self._handle = handle

    def wait(self):
        return self._client.Supervisor.Wait(handle=self._handle)

    def signal(self, signal):
        return self._client.Supervisor.Signal(handle=self._handle, signal=signal)

class Client(object):
    def __init__(self):
        self.context = load_expando(os.environ['K4_CONTEXT'])

        self._id = 0
        self._socket = socket.fromfd(3, socket.AF_UNIX, socket.SOCK_STREAM)

    def _readline(self):
        buf = []
        while True:
            c = self._socket.recv(1)
            if c == b'':
                raise EOFError
            buf.append(c)
            if c == b'\n':
                return b''.join(buf)

    def _send_fds(self, fds):
        return self._socket.sendmsg([b'\1'], [(socket.SOL_SOCKET, socket.SCM_RIGHTS, array.array("i", fds))])

    def spawn(self, owner_name, name, stdin, stdout, stderr):
        if hasattr(stdin, 'fileno'):
            stdin = stdin.fileno()

        if hasattr(stdout, 'fileno'):
            stdout = stdout.fileno()

        if hasattr(stderr, 'fileno'):
            stderr = stderr.fileno()

        handle = self.Supervisor.Spawn(after_send=lambda: self._send_fds([stdin, stdout, stderr]), owner_name=owner_name, name=name).handle
        return Child(self, handle)

    def call(self, method, after_send=None, **kwargs):
        req_id = self._id

        self._socket.send(json.dumps({
            'id': req_id,
            'method': method,
            'params': [kwargs],
        }).encode('utf-8') + b'\n')

        if after_send is not None:
            after_send()

        self._id += 1

        while True:
            resp = load_expando(self._readline().decode('utf-8'))

            if resp.id < req_id:
                continue

            if resp.id > req_id:
                raise MismatchedIDError('expected {}, got {}'.format(
                    req_id, resp.id))

            if resp.error is not None:
                raise ServerError(resp.error)

            return resp.result

    def __getattr__(self, name):
        return _Stub(self, name)
