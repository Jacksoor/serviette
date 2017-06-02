Networking
==========

kobun4 uses Linux virtual "veth" Ethernet devices for networking. In order to use veth, some configuration is required.

Make sure IP forwarding is enabled in ``/etc/sysctl.conf``:

.. code-block:: ini

   net.ipv4.ip_forward=1

Make sure there is an appropriate bridge interface. For Debian/Ubuntu, one can be configured inside ``/etc/network/interfaces``:

.. code-block:: none

   auto br0
   iface br0 inet static
       address 10.0.0.1
       netmask 255.255.255.0
       network 10.0.0.0
       broadcast 10.0.0.255

iptables should also be set up to allow NAT:

.. code-block:: bash

   iptables -t nat -A POSTROUTING -o br0 -j MASQUERADE
   iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
