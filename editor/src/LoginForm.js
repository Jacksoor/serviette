import React, { Component } from 'react';
import { Alert, Button, Input } from 'reactstrap';

export default class LoginForm extends Component {
  constructor(props) {
    super(props);
    this.state = {
      disabled: false,
      lastError: null
    };
  }

  onSubmit(e) {
    e.preventDefault();
    var username = e.target.elements.username.value;
    var password = e.target.elements.password.value;
    this.setState({lastError: null, disabled: true});
    this.props.client.login(username, password).then((token) => {
      this.props.onLoggedIn(username);
    }, (error) => {
      this.setState({lastError: 'Login failed: ' + error, disabled: false});
    });
  }

  render() {
    return (
      <div className="login bg-faded">
        <form onSubmit={this.onSubmit.bind(this)}>
          <fieldset disabled={this.state.disabled}>
            <h2 className="text-muted">Please log in</h2>
            {this.state.lastError ? <Alert color="danger">{this.state.lastError}</Alert> : null}
            <Input name="username" className="username" type="text" placeholder="Username" maxLength="20" />
            <Input name="password" className="password" type="password" placeholder="Password" />
            <Button block color="primary" size="lg">Log In</Button>
          </fieldset>
        </form>
      </div>
    );
  }
}
