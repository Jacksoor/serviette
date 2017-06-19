.. _sandbox:

Sandbox
=======

All kobun4 commands are run in an `nsjail <https://github.com/google/nsjail>`_ sandbox. Scripts are subject to various restrictions, depending on the user who owns the script:

 * A clean chroot, independent of the host.

 * A maximum memory limitation.

 * A maximum time limit.

 * A single persistent storage area in :ref:`persistentstorage`.

 * A single ephemeral storage area in :ref:`ephemeralstorage`.

 * Throttled network access, if permitted.
