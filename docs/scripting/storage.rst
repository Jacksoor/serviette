Storage
=======

The kobun4 scripting environment provides both ephemeral and persistent storage.

.. _persistentstorage:

``/mnt/storage``
----------------

``/mnt/storage`` is the persistent storage location on a per-account basis. It is limited in size and is guaranteed to be persistent from one script execution to the next.

Persistent storage can be accessed via WebDAV at the URL https://storage.kobun.life using an account handle as the username and its key as the password.

.. _ephemeralstorage:

``/tmp``
--------

``/tmp`` is the ephemeral storage location. It is limited in size and will be wiped from one script execution to the next.

``/mnt/scripts``
----------------

``/mnt/scripts`` is a read-only mount containing all scripts, with ``<account handle>/<script name>`` paths.
