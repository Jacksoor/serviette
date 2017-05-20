import json
import socket
import sys


class RPCError(Exception):
    pass


class ClientError(Exception):
    pass


class MismatchedIDError(Exception):
    pass


class ServerError(Exception):
    pass


class Client(object):
    def __init__(self):
        self._id = 0
        self._f = socket.fromfd(3, socket.AF_UNIX, socket.SOCK_STREAM).makefile(
            'rwb', buffering=0)
        self.context = json.load(sys.stdin)

    def call(self, method, **kwargs):
        req_id = self._id

        self._f.write(json.dumps({
            'id': req_id,
            'method': method,
            'params': [kwargs],
        }).encode('utf-8'))
        self._id += 1

        while True:
            raw = json.loads(self._f.readline())

            resp_id = raw['id']
            error = raw['error']

            if resp_id < req_id:
                continue

            if resp_id > req_id:
                raise MismatchedIDError('expected {}, got {}'.format(
                    req_id, resp_id))

            if error is not None:
                raise ServerError(error)

            return raw['result']
