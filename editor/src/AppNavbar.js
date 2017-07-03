import React, { Component } from 'react';
import { Button, Collapse, InputGroup, InputGroupAddon, InputGroupButton, Modal, ModalBody, ModalHeader, Nav, Navbar, NavbarBrand, NavbarToggler, NavItem } from 'reactstrap';

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

  render() {
    const account = this.props.account;
    return (
      <div>
        <Navbar color="faded" light toggleable>
          <NavbarToggler right onClick={this.toggleNavbar} />
          <NavbarBrand href="/">Kobun<small><sup>(β)</sup></small> Editor</NavbarBrand>
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
            <ModalBody><pre>{JSON.stringify(this.props.account.info, null, 2)}</pre></ModalBody>
          </Modal> :
          null
        }
      </div>
    );
  }
}
