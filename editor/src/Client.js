export default class Client {
  constructor() {
    this.logout();
  }

  _request(method, path, payload) {
    return new Promise((resolve, reject) => {
      var xhr = new XMLHttpRequest();
      xhr.onload = function () {
        if (xhr.status === 200 || xhr.status === 202) {
          resolve(xhr.responseText === '' ? undefined : JSON.parse(xhr.responseText));
        } else {
          reject(xhr.responseText);
        }
      };
      xhr.onerror = function (e) {
        reject(e);
      };
      xhr.open(method, '/api/v1/' + path);
      xhr.setRequestHeader('Content-Type', 'application/json');
      xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
      if (this.token !== null) {
        xhr.setRequestHeader('Authorization', 'Bearer ' + this.token);
      }
      xhr.send(method === 'POST' || method === 'PUT' ? JSON.stringify(payload) : null);
    });
  }

  userpassLogin(username, password) {
    return this._request('POST', 'login/userpass', {
      username: username,
      password: password,
    }).then(r => {
      this.username = username;
      this.token = r.token;
      return r.token;
    });
  }

  getAccountInfo() {
    return this._request('GET', 'accounts/' + this.username).then(r => r.info);
  }

  getAccountScripts() {
    return this._getAccountScripts(0, 50);
  }

  _getAccountScripts(offset, limit) {
    return this._request('GET', 'scripts/' + this.username + '?offset=' + offset + '&limit=' + limit).then(scripts => {
      if (scripts.length >= limit) {
        return this._getAccountScripts(offset + limit, limit).then(nextScripts => {
          return scripts.concat(nextScripts);
        });
      } else {
        return scripts;
      }
    });
  }

  getScriptContent(name) {
    return this._request('GET', 'scripts/' + this.username + '/' + name + '?getContent=true').then(r => r.content);
  }

  createScript(script) {
    return this._request('POST', 'scripts', script);
  }

  putScript(script) {
    return this._request('PUT', 'scripts/' + this.username + '/' + script.name, script).then(r => r.content);
  }

  deleteScript(name) {
    return this._request('DELETE', 'scripts/' + this.username + '/' + name);
  }

  logout() {
    this.username = null;
    this.token = null;
  }
}
