var net = require('net');

function Client() {
    this.context = JSON.parse(process.env.K4_CONTEXT);

    this._id = 0;
    this._pending_id = null;

    this._socket = new net.Socket({fd: 3});
    this._socket.on('data', this._onSocketData.bind(this));
    this._socket.on('error', this._onSocketError.bind(this));

    this._cb = null;

    this._buf = '';
}

Client.prototype.call = function (method, req, cb) {
    if (this._pending_id !== null) {
        cb(new Error('existing request currently pending'));
        return;
    }

    this._pending_id = this._id;
    this._cb = cb;
    this._socket.write(JSON.stringify({
        id: this._pending_id,
        method: method,
        params: [req]
    }));
    ++this._id;
};

Client.prototype._runCallback = function (err, res) {
    this._pending_id = null;
    var cb = this._cb;
    this._cb = null;
    cb(err, res);
}

Client.prototype._onSocketData = function (chunk) {
    this._buf += chunk;
    var i = this._buf.indexOf('\n');
    if (i !== -1) {
        var raw = this._buf.substring(0, i);
        this._buf = this._buf.substring(i + 1);

        try {
            var resp = JSON.parse(raw);
        } catch (e) {
            this._runCallback(e);
            return;
        }

        if (resp.id < this._pending_id) {
            return;
        }

        if (resp.id > this._pending_id) {
            this._runCallback(new Error('mismatched id: expected ' + this._pending_id + ', got ' + resp.id));
            return;
        }

        if (resp.error !== null) {
            this._runCallback(new Error(resp.error));
            return;
        }

        this._runCallback(null, resp.result);
    }
};

Client.prototype._onSocketError = function (e) {
    if (this._cb !== null) {
        this._runCallback(e);
    }
};

Client.prototype.close = function () {
    this._socket.destroy();
};

exports.Client = Client;
