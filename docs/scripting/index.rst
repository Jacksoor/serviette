Scripting Documentation
=======================

kobun4's non-bank commands (generally prefixed with ``!``) are all user scriptable.

Scripts can be written via the web interface and are run in a :ref:`sandbox <sandbox>`, which has certain limitations you should be aware of. They can be written in any executable format, and will be directly ``exec``\ed by the executor in the sandbox (so a ``#!`` is required for script interpreters).

When a user executes a script (either via ``!command arg`` or ``$escrow amount command arg``), a script is ``exec``\ed in the sandbox and the argument sent to its stdin. Its stdout and stderr, up to a limit (generally 5MB each) are captured and sent to the entity requesting execution, as well as exit code.

 * On exit code 0, the executing entity will report success, along with the contents of stdout (success).
 * On any exit code other than 2, the executing entity will report failure, along with the contents of stderr (script failure).
 * On exit code 2, the executing entity will report failure, along with the contents of stdout (script error).

Scripts are subject to :ref:`billing <billing>`, but usage may be optionally billed to the owner of the script.

.. toctree::

   clients
   bridges
   context
   services
   storage
   aliases
