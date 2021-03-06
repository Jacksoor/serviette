Storage
=======

The kobun4 scripting environment provides both ephemeral and persistent storage.

.. _persistentstorage:

``/mnt/private``
----------------

``/mnt/private`` is the persistent storage location on a per-account basis. It is limited in size and is guaranteed to be persistent from one script execution to the next.

Persistent storage can be accessed via WebDAV at the URL https://storage.kobun.company.

.. _ephemeralstorage:

``/tmp``
--------

``/tmp`` is the ephemeral storage location. It is limited in size and will be wiped from one script execution to the next.

``/mnt/scripts``
----------------

``/mnt/scripts`` is a read-only mount containing all scripts, with ``<account handle>/<script name>`` paths.
