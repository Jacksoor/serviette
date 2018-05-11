.. _scripting:

Scripting Documentation
=======================

kobun4's commands (generally prefixed with ``.``) are all scriptable via the web UI at https://kobun.company/editor.

Scripts can be written via the web interface and are run in a :ref:`sandbox <sandbox>`, which has certain limitations you should be aware of. They can be written in any executable format, and will be directly ``exec``\ed by the executor in the sandbox (a ``#!`` on the first line is required for specifying the script interpreter).

When a user executes a script (via ``.<command name>``), a script is ``exec``\ed in the sandbox and the argument sent to its stdin. Its stdout and stderr, up to a limit (generally 5MB each) are captured and sent to the bridge requesting execution, as well as exit code.

 * On exit code 0, the bridge will report success, along with the contents of stdout (success).

 * On any exit code other than 2, the bridge will report failure, along with the contents of stderr (script failure).

 * On exit code 2, the bridge will report failure, along with the contents of stdout (script error).

 * On SIGKILL, the bridge will report that execution took too long.

 * On any other signal, the bridge will report the signal details.

.. toctree::
   :caption: Topics

   clients
   bridges
   context
   services
   storage
