Clients
=======

The scripting environment comes with a set of pre-written clients for easier interaction with the scripting API in ``/usr/lib/k4``.

Python
------

.. code-block:: python

   import sys
   sys.path.insert(0, '/usr/lib/k4')
   import k4

   client = k4.Client()


All members of the :ref:`context <context>` are exposed via ``client.context``.

:ref:`Services <services>` can be accessed directly via members on the client object, e.g.:

.. code-block:: python

   user_name = client.Bridge.GetUserInfo(id=...).name
