Networking
==========

kobun4 uses Linux virtual "veth" Ethernet devices for networking. In order to use veth, some configuration is required.

Make sure IP forwarding is enabled in ``/etc/sysctl.conf``:

.. code-block:: ini

   net.ipv4.ip_forward=1

Make sure there are appropriate bridge and veth interfaces. For Debian/Ubuntu, these can be configured inside ``/etc/network/interfaces`` (with bandwidth throttling managed by wondershaper):

.. code-block:: none

   auto br0
   iface br0 inet static
       bridge_ports none
       address 10.0.0.1
       netmask 255.255.255.0
       network 10.0.0.0
       broadcast 10.0.0.255

   auto veth0
   iface veth0 inet manual
       pre-up ip link add dev veth0 type veth peer name veth1
       pre-up ip link set dev veth0 master br0
       up ip link set dev veth0 up
       up ip link set dev veth1 up
       up wondershaper veth0 1024 1024
       down wondershaper remove veth0
       down ip link set dev veth0 down
       post-down ip link del veth0

iptables should also be set up to allow NAT:

.. code-block:: bash

   iptables -t nat -A POSTROUTING -o br0 -j MASQUERADE
   iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
