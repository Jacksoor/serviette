import React, { Component } from 'react';
import { Button, Input, InputGroup, InputGroupAddon, InputGroupButton, Label } from 'reactstrap';
import CodeMirror from '@skidding/react-codemirror';
import 'codemirror/lib/codemirror.css';

import 'codemirror/mode/javascript/javascript';
import 'codemirror/mode/lua/lua';
import 'codemirror/mode/python/python';
import 'codemirror/mode/shell/shell';

import 'codemirror/addon/edit/matchbrackets';

const KNOWN_SYNTAXES = {
  '/bin/bash': 'shell',
  '/bin/sh': 'shell',
  '/usr/bin/python3': 'python',
  '/usr/bin/lua': 'lua',
  '/usr/bin/node': 'javascript',
};

export default class Editor extends Component {
  onSubmit(e) {
    e.preventDefault();
    this.save();
  }

  save() {
    this.props.onSave({
      ownerName: this.props.script.ownerName,
      name: this.props.script.name,
      visibility: parseInt(this.refs.form.elements.visibility.value, 10),
      description: this.refs.form.elements.description.value,
      content: this.refs.codemirror.getCodeMirror().getValue(),
    });
  }

  onDelete() {
    if (confirm('Are you sure you want to delete this script?')) {
      this.props.onDelete(this.props.script.name);
    }
  }

  onDescriptionChanged(e) {
    this.props.onChange();
  }

  onPublishedChanged(e) {
    this.props.onChange();
  }

  onContentChanged(content) {
    this.props.onChange();
  }

  updateSyntax() {
    const cm = this.refs.codemirror.getCodeMirror();

    const line1 = cm.getLine(0);

    if (line1.substring(0, 2) !== '#!') {
      cm.setOption('mode', null);
      return;
    }

    let prog = line1.substring(2);
    const spaceIndex = prog.indexOf(' ');
    if (spaceIndex !== -1) {
      prog = prog.substring(0, spaceIndex);
    }

    cm.setOption('mode', KNOWN_SYNTAXES[prog] || null);
  }

  componentDidMount() {
    const cm = this.refs.codemirror.getCodeMirror();

    this.codemirrorChangeCallback = (cm, changes) => {
      if (!changes.some(change => change.from.line === 0)) {
        return;
      }

      this.updateSyntax();
    };
    this.updateSyntax();

    cm.on('changes', this.codemirrorChangeCallback);
  }

  componentWillUnmount() {
    const cm = this.refs.codemirror.getCodeMirror();
    cm.off('changes', this.codemirrorChangeCallback);
  }

  render() {
    return (
      <form ref="form" className="inner" onSubmit={this.onSubmit.bind(this)}>
        <InputGroup className="toolbar">
          <Input name="description" defaultValue={this.props.script.description} placeholder="Description" onChange={this.onDescriptionChanged.bind(this)} maxLength="100" />
          <InputGroupAddon>
            <select name="visibility" defaultValue={this.props.script.visibility}>
              <option value="0">Unpublished</option>
              <option value="1">Unlisted</option>
              <option value="2">Published</option>
            </select>
          </InputGroupAddon>
          <InputGroupButton><Button ref="saveButton" type="submit" color="primary">Save</Button></InputGroupButton>
          <InputGroupButton><Button type="button" color="danger" onClick={this.onDelete.bind(this)}>Delete</Button></InputGroupButton>
        </InputGroup>
        <div className="editor" id="editor">
          <CodeMirror ref="codemirror" defaultValue={this.props.script.content} onChange={this.onContentChanged.bind(this)} options={{
            lineNumbers: true,
            viewportMargin: Infinity,
            indentUnit: 4,
            tabSize: 4,
            indentWithTabs: false,
            matchBrackets: true,
            lineWrapping: true,
            extraKeys: {
              Tab: () => {
                const cm = this.refs.codemirror.getCodeMirror();
                if (cm.somethingSelected()) {
                  cm.indentSelection("add");
                } else {
                  cm.replaceSelection(cm.getOption("indentWithTabs") ? "\t" : Array(cm.getOption("indentUnit") + 1).join(" "), "end", "+input");
                }
              },

              'Cmd-S': () => {
                this.save();
              },

              'Ctrl-S': () => {
                this.save();
              }
            }
          }}/>
        </div>
      </form>
    );
  }
}
