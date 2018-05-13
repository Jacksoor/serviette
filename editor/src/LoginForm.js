import React, { Component } from 'react';
import { Alert, Button, Input } from 'reactstrap';

export default class LoginForm extends Component {
  constructor(props) {
    super(props);
    this.state = {
      lastError: null,
      disabled: false,
      discordToken: null,
    };
  }

  componentDidMount() {
    window.onDiscordToken = (token) => {
      this.props.client.discordLogin(token).then((username) => {
        this.props.onLoggedIn(username);
      }, (error) => {
        if (error.code === 404) {
          this.setState({
            lastError: null,
            disabled: false,
            discordToken: token,
          });
        } else {
          this.setState({
            lastError: error,
            disabled: false
          });
        }
      });
    };
  }

  onSubmit(e) {
    e.preventDefault();

    var username = e.target.elements.username.value;

    if (this.state.discordToken !== null) {
      this.props.client.discordLogin(this.state.discordToken, username).then((username) => {
        this.props.onLoggedIn(username);
      }, (error) => {
        this.setState({
          lastError: error,
          disabled: false
        });
      });
    }

    var password = e.target.elements.password.value;
    this.setState({lastError: null, disabled: true});
    this.props.client.userpassLogin(username, password).then(() => {
      this.props.onLoggedIn(username);
    }, (error) => {
      this.setState({
        lastError: error,
        disabled: false
      });
    });
  }

  onDiscordLogin(e) {
    window.open("https://discordapp.com/oauth2/authorize?client_id=" + encodeURIComponent(process.env.DISCORD_CLIENT_ID) + "&scope=identify&redirect_uri=" + encodeURIComponent(window.location + '_oauthcallback.html') + "&access_type=offline&response_type=token", "", "width=600,height=600");
  }

  render() {
    if (this.state.discordToken !== null) {
      return (
        <div className="login bg-faded text-center">
          <form onSubmit={this.onSubmit.bind(this)}>
            <fieldset disabled={this.state.disabled}>
              <h2 className="text-muted">Please select a username</h2>
              <p>This username cannot be changed, <strong>ever</strong>, so choose carefully!</p>
              <p>Only numbers and lowercase alphabetical characters are allowed.</p>
              {this.state.lastError ? <Alert color="danger">{this.state.lastError.toString()}</Alert> : null}
              <Input name="username" className="username" type="text" placeholder="Username" maxLength="20" />
              <Button block color="primary" size="lg">Register</Button>
            </fieldset>
          </form>
        </div>
      );
    }

    return (
      <div className="login bg-faded text-center">
        <form onSubmit={this.onSubmit.bind(this)}>
          <fieldset disabled={this.state.disabled}>
            <h2 className="text-muted">Please log in</h2>
            {/Trident\/|MSIE /.test(window.navigator.userAgent) ? <Alert color="danger">The editor is known to be broken on all versions of Internet Explorer. Use at your own risk!</Alert> : null}
            {this.state.lastError ? <Alert color="danger">{this.state.lastError.toString()}</Alert> : null}
            <Input name="username" className="username" type="text" placeholder="Username" maxLength="20" />
            <Input name="password" className="password" type="password" placeholder="Password" />
            <Button block color="primary" size="lg">Log In</Button>
            <div>or</div>
            <Button onClick={this.onDiscordLogin.bind(this)} block type="button" color="primary" style={{backgroundColor: "#7289da", borderColor: "#7289da"}}>Log in with <img src={process.env.PUBLIC_URL + "/discord_logo.svg"} alt="Discord" style={{height: "1.75rem", margin: "-0.75rem 0"}} /></Button>
          </fieldset>
        </form>
      </div>
    );
  }
}
