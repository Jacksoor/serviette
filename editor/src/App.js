import React, { Component } from 'react';
import './App.css';

import Client from './Client';

import AppNavbar from './AppNavbar';
import AppSidebar from './AppSidebar';
import Editor from './Editor';
import LoginForm from './LoginForm';

function makeDefaultScript(ownerName, name) {
  return `#!/usr/bin/python3

# To run this command, run \`@Kobun run ${ownerName}/${name}\` in a server with Kobun that you are the administrator of.

print("Hello world!")
`;
}

export default class App extends Component {
  constructor(props) {
    super(props);

    this.state = {
      account: null,
      currentScript: null,
      unsaved: false,
      client: new Client(),
    };
  }

  componentWillMount() {
    window.onbeforeunload = () => {
      if (this.state.unsaved) {
        return "";
      }
    };
  }

  onLoggedIn(username) {
    Promise.all([
      this.state.client.getAccountInfo(),
      this.state.client.getAccountScripts()
    ]).then(r => {
      const [info, scripts] = r;
      this.setState({account: {
        name: username,
        info: info,
        scripts: scripts,
      }});
    }, e => {
      alert(`Failed to get user details: ${e}. Please log out and log in again.`)
    });
  }

  updateScriptsList() {
    return this.state.client.getAccountScripts().then(scripts => {
      this.setState({account: {...this.state.account, scripts: scripts}});
    }, e => {
      alert(`Failed to update scripts list: ${e}. Please log out and log in again.`)
    })
  }

  logout() {
    this.state.client.logout();
    this.setState({
      account: null,
      currentScript: null,
    });
  }

  onScriptCreate(name) {
    const script = {
      ownerName: this.state.account.name,
      name: name,
      published: false,
      description: '',
      content: makeDefaultScript(this.state.account.name, name)
    };
    this.state.client.createScript(script).then(script => {
      this.updateScriptsList().then(() => {
        this.onScriptSelected(script);
      });
    }, e => {
      alert(`Failed to create script: ${e}. Please log out and log in again.`)
    });
  }

  onScriptSelected(script) {
    if (this.state.unsaved && !confirm("Your current changes haven't been saved yet. Are you sure you want to switch to a new script?")) {
      return;
    }

    this.state.client.getScriptContent(script.name).then(content => {
      this.setState({
        currentScript: {...script, content: content},
        unsaved: false,
      });
    }, e => {
      alert(`Failed to get script: ${e}. Please log out and log in again.`)
    });
  }

  onChange() {
    this.setState({unsaved: true});
  }

  onSave(script) {
    this.state.client.putScript(script).then(() => {
      this.updateScriptsList().then(() => {
        this.setState({unsaved: false});
      });
    }, e => {
      alert(`Failed to save script: ${e}. Please copy and paste your script somewhere, log out, and log in again.`)
    });
  }

  onDelete(name) {
    this.state.client.deleteScript(name).then(() => {
      this.updateScriptsList().then(() => {
        this.setState({
          currentScript: null,
          unsaved: false,
        });
      })
    }, e => {
      alert(`Failed to delete script: ${e}. Please log out and log in again.`)
    });
  }

  render() {
    return (
      <div className="app">
        <AppNavbar account={this.state.account} onLogout={this.logout.bind(this)} />

        {this.state.account !== null ?
          <div className="outer">
            <AppSidebar account={this.state.account} currentScript={this.state.currentScript} onScriptSelected={this.onScriptSelected.bind(this)} onCreate={this.onScriptCreate.bind(this)} unsaved={this.state.unsaved} />
            {this.state.currentScript !== null ?
              <Editor script={this.state.currentScript} onChange={this.onChange.bind(this)} onSave={this.onSave.bind(this)} onDelete={this.onDelete.bind(this)} key={this.state.currentScript.name} /> :
              <div className="landing">
                <p>Please select or create a script to begin editing.</p>
                <p>Not sure how it all works? Check out the <a target="_blank" href="https://docs.kobun.life/en/latest/scripting/index.html">scripting documentation</a> or the <a target="_blank" href="/scripts">script library</a> for ideas!</p>
              </div>}
          </div> :
          <LoginForm client={this.state.client} onLoggedIn={this.onLoggedIn.bind(this)} />
        }
      </div>
    );
  }
}
