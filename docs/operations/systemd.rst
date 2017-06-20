systemd Configuration
=====================

kobun4 uses systemd units for daemon configuration. If you have installed kobun4 into ``/opt/kobun4``, the units are available in the ``systemd`` subdirectory.

``systemctl`` can be used to enable the units:

.. code-block:: bash

    systemctl enable /opt/kobun4/systemd/kobun4-executor.service
    systemctl enable /opt/kobun4/systemd/kobun4-discordbridge.service

Service configurations are stored in `/etc/kobun4` as each component's name. Please consult the unit files for the applicable environment variables.

The unit files specify that each component is run under a POSIX user with the same name as the component (e.g. ``executor`` runs under the ``kobun4-executor`` user).
