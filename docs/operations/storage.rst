Storage
=======

kobun4 recommends that quotas should be set up for user storage locations to prevent any one user from using all available disk space. Any file system with per-directory quotas can be used, but ZFS is the most straightforward.

You must first create a storage pool for all user storage:

.. code-block:: bash

   zpool create kobun4-executor-storage <path to disk/image> -m /var/lib/kobun4/executor/storage

For each registered user, you must also create their directory and set their quota:

.. code-block:: bash

   zfs create kobun4-executor-storage/exampleuser
   zfs set quota=20M kobun4-executor-storage/exampleuser
