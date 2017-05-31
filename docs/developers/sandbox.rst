.. _sandbox:

Sandbox
=======

All kobun4 commands are run in an nsjail_ sandbox. Scripts are subject to:

 * A clean chroot, independent of the host.

 * A cgroup memory limitation (generally 20MB).

 * A maximum time limit of 5 seconds.

 * A single persistent storage area in :ref:`persistentstorage`.

 * A single ephemeral storage area in :ref:`ephemeralstorage`.

.. _nsjail: https://github.com/google/nsjail
