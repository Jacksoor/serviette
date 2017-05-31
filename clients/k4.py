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


class Client(object):
    def __init__(self):
        self.context = load_expando(os.environ['K4_CONTEXT'])

        self._id = 0
        self._f = socket.fromfd(3, socket.AF_UNIX, socket.SOCK_STREAM).makefile(
            'rwb')

    def call(self, method, **kwargs):
        req_id = self._id

        self._f.write(json.dumps({
            'id': req_id,
            'method': method,
            'params': [kwargs],
        }).encode('utf-8') + b'\n')
        self._f.flush()

        self._id += 1

        while True:
            resp = load_expando(self._f.readline().decode('utf-8'))

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
