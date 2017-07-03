import React, { Component } from 'react';
import { Button, Form, Nav, NavItem, NavLink, Input, InputGroup, InputGroupButton } from 'reactstrap';

export default class AppSidebar extends Component {
  onCreate(e) {
    e.preventDefault();
    this.props.onCreate(e.target.elements.name.value);
    e.target.elements.name.value = '';
  }

  render() {
    return (
      <div className="bg-faded sidebar">
        <Form className="p-3" onSubmit={this.onCreate.bind(this)}>
          <InputGroup>
            <Input type="text" placeholder="Script name" name="name" />
            <InputGroupButton>
              <Button type="submit" color="primary">Create</Button>
            </InputGroupButton>
          </InputGroup>
        </Form>
        <Nav vertical pills>
          {this.props.account.scripts.map((script) => {
            const current = this.props.currentScript !== null && this.props.currentScript.name === script.name;
            return <NavItem key={script.name}><NavLink href="javascript:;" className={script.published ? null : "text-muted"} active={current} onClick={this.props.onScriptSelected.bind(this, script)}><samp>{script.name}</samp>{this.props.unsaved && current ? <sup>*</sup> : ''}</NavLink></NavItem>;
          })}
        </Nav>
      </div>
      );
    }
}
