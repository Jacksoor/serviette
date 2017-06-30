Networking
==========

kobun4 uses Linux virtual "veth" Ethernet devices for networking. In order to use veth, some configuration is required.

Make sure IP forwarding is enabled in ``/etc/sysctl.conf``:

.. code-block:: ini

   net.ipv4.ip_forward=1

Make sure there are appropriate bridge and veth interfaces, with one half of the veth interface in a network namespace named ``kobun4``. For Debian/Ubuntu, these can be configured inside ``/etc/network/interfaces`` (with bandwidth throttling managed by wondershaper):

.. code-block:: none

   auto br0
   iface br0 inet static
       bridge_ports none
       address 10.0.0.1
       netmask 255.255.255.0
       network 10.0.0.0
       broadcast 10.0.0.255

   auto hostveth0
   iface hostveth0 inet manual
       pre-up ip link add dev hostveth0 type veth peer name guestveth0
       pre-up ip link set dev hostveth0 master br0
       pre-up ip netns add kobun4
       pre-up ip link set guestveth0 netns kobun4
       up ip link set dev hostveth0 up
       up ip netns exec kobun4 ip link set dev lo up
       up ip netns exec kobun4 ip link set dev guestveth0 up
       post-up ip netns exec kobun4 ip addr add 10.0.0.2/8 broadcast 10.255.255.255 dev guestveth0
       post-up ip netns exec kobun4 ip route add default via 10.0.0.1
       post-up wondershaper hostveth0 1024 1024
       down wondershaper remove hostveth0
       down ip link set dev hostveth0 down
       post-down ip link del hostveth0
       post-down ip netns del kobun4

iptables should also be set up to allow NAT:

.. code-block:: bash

   iptables -t nat -A POSTROUTING -o br0 -j MASQUERADE
   iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE

To allow Kobun to use network namespaces, you must also grant the executor user permission to run ``nsenter`` for the namespace. In your sudoers file, which you can edit using ``visudo``, add:

.. code-block:: none

   kobun4-executor ALL=(ALL) NOPASSWD: /usr/bin/nsenter -n/run/netns/kobun4 sudo -u kobun4-executor *
