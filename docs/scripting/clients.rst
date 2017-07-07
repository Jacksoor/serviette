Clients
=======

The scripting environment comes with a set of pre-written clients for easier interaction with the scripting API in ``/usr/lib/k4``.

Python
------

.. code-block:: python

   #!/usr/bin/python3

   import sys
   sys.path.insert(0, '/usr/lib/k4')
   import k4

   client = k4.Client()


All members of the :ref:`context <context>` are exposed via ``client.context``.

:ref:`Services <services>` can be accessed directly via members on the client object, e.g.:

.. code-block:: python

   user_name = client.Bridge.GetUserInfo(id='example').name

Lua
---

.. code-block:: lua

   #!/usr/bin/lua

   package.path = '/usr/lib/k4/?.lua;' .. package.path
   k4 = require('k4')

   client = k4.Client.new()

The Lua client is experimental.

All members of the :ref:`context <context>` are exposed via ``client.context``.

:ref:`Services <services>` can be accessed via the `call` method, e.g.:

.. code-block:: lua

   user_name = client.call('Bridge.GetUserInfo', {id='example'}).name

JavaScript (Node.js)
--------------------

.. code-block:: javascript

   #!/usr/bin/node

   const k4 = require('/usr/lib/k4/k4');

   const client = new k4.Client();

The Node.js client is experimental.

All members of the :ref:`context <context>` are exposed via ``client.context``.

:ref:`Services <services>` can be accessed via the `call` method, e.g.:

.. code-block:: javascript

   client.call('Bridge.GetUserInfo', {id: 'example'}, function (err, resp) {
      var user_name = resp.name;
   });

.. warning:: You must call ``client.close()`` when you are done with the client, or Node.js will hang indefinitely.
