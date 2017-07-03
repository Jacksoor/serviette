import React, { Component } from 'react';
import './App.css';

import Client from './Client';

import AppNavbar from './AppNavbar';
import AppSidebar from './AppSidebar';
import Editor from './Editor';
import LoginForm from './LoginForm';

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
    })
  }

  updateScriptsList() {
    return this.state.client.getAccountScripts().then(scripts => {
      this.setState({account: {...this.state.account, scripts: scripts}});
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
      content: 'CONTENT'
    };
    this.state.client.createScript(script).then(() => {
      this.updateScriptsList().then(() => {
        this.onScriptSelected(script);
      });
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
                <p>Please select a script to begin editing.</p>
              </div>}
          </div> :
          <LoginForm client={this.state.client} onLoggedIn={this.onLoggedIn.bind(this)} />
        }
      </div>
    );
  }
}
