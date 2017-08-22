import React, { Component } from 'react';
import { Button, Collapse, Form, FormGroup, Input, InputGroup, InputGroupAddon, InputGroupButton, Modal, ModalBody, ModalHeader, Nav, Navbar, NavbarBrand, NavbarToggler, NavItem } from 'reactstrap';

export default class AppNavbar extends Component {
  constructor(props) {
    super(props);

    this.toggleNavbar = this.toggleNavbar.bind(this);
    this.state = {
      isOpen: false,
      isAccountModalOpen: false,
    };
  }

  toggleNavbar() {
    this.setState({
      isOpen: !this.state.isOpen
    });
  }

  toggleAccountInfoModal() {
    this.setState({
      isAccountModalOpen: !this.state.isAccountModalOpen
    })
  }

  onSetPassword(e) {
    e.preventDefault();

    var newPassword = e.target.elements.newPassword.value;
    var confirmPassword = e.target.elements.confirmPassword.value;

    if (newPassword !== confirmPassword) {
      alert('Passwords are not the same!');
      return;
    }

    this.props.onSetPassword(newPassword);
    e.target.elements.newPassword.value = '';
    e.target.elements.confirmPassword.value = '';
  }

  render() {
    const account = this.props.account;
    return (
      <div>
        <Navbar color="faded" light toggleable>
          <NavbarToggler right onClick={this.toggleNavbar} />
          <NavbarBrand>Kobun<small><sup>(β)</sup></small> Editor</NavbarBrand>
          <Collapse isOpen={this.state.isOpen} navbar>
            <Nav className="ml-auto" navbar>{account !== null ?
              <NavItem>
                <InputGroup>
                  <InputGroupAddon>{account.name}</InputGroupAddon>
                  <InputGroupButton><Button color="secondary" onClick={this.toggleAccountInfoModal.bind(this)}>Account Info</Button></InputGroupButton>
                  <InputGroupButton><Button color="primary" onClick={this.props.onLogout}>Log Out</Button></InputGroupButton>
                </InputGroup>
              </NavItem> :
              null
            }</Nav>
          </Collapse>
        </Navbar>
        {account !== null ?
          <Modal isOpen={this.state.isAccountModalOpen} toggle={this.toggleAccountInfoModal.bind(this)}>
            <ModalHeader toggle={this.toggleAccountInfoModal.bind(this)}>Account Info</ModalHeader>
            <ModalBody>
              <pre>{JSON.stringify(this.props.account.info, null, 2)}</pre>
              <Form onSubmit={this.onSetPassword.bind(this)}>
                <FormGroup>
                  <Input type="password" name="newPassword" placeholder="New Password" />
                </FormGroup>
                <FormGroup>
                  <Input type="password" name="confirmPassword" placeholder="Confirm Password" />
                </FormGroup>
                <Button color="primary">Set Password</Button>
              </Form>
            </ModalBody>
          </Modal> :
          null
        }
      </div>
    );
  }
}