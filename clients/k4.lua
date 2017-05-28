json = require('dkjson')
socket = require('posix.sys.socket')

Client = {}
Client.__index = Client

function Client.new()
    local self = setmetatable({}, Client)

    self._id = 0
    self._socket = 3

    return self
end

function Client:_sendall(buf)
    while #buf > 0 do
        local err
        buf, err = buf:sub(socket.send(self._socket, buf) + 1)
        if err ~= nil then
            error(err)
        end
    end
end

function Client:_readline()
    local buf = {}
    while true do
        local c = socket.recv(self._socket, 1)
        table.insert(buf, c)
        if c == "\n" then
            return table.concat(buf, "")
        end
    end
end

function Client:call(method, req)
    local reqId = self._id
    self:_sendall(json.encode({
        id = reqId,
        method = method,
        params = {setmetatable(req, {__jsontype = "object"})},
    }) .. "\n")
    self._id = self._id + 1

    while true do
        local raw, _, err = json.decode(self:_readline())
        if err ~= nil then
            error(err)
        end

        if raw.id > reqId then
            error(string.format("mismatched id: expected %d, got %d", reqId, raw.id))
        end

        if raw.id == reqId then
            if raw.error ~= nil then
                error(raw.error)
            end

            return raw.result
        end
    end
end

return {
    Client = Client,
}
