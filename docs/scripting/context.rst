.. _context:

Context
=======

The *context* contains immutable information about the state of the world in which the script was executed in.

It is contained in the environment variable ``K4_CONTEXT``, marshaled as JSON.

The following fields are always available:

.. py:data:: bridgeName

   The name of the chat service or otherwise entity that requested execution of the script (the *bridge*).

.. py:data:: commandName

   The name of the command used to run the script.

.. py:data:: userId

   The ID of the user who requested the command to be executed.

   .. warning:: ``userId`` does not completely uniquely identify a user. In order to identify a user completely, it must be used in combination with ``networkId`` and ``bridgeName``, e.g. in the form ``bridgeName/networkId/userId``.

.. py:data:: channelId

   The ID of the channel the command was executed on.

.. py:data:: groupId

   The ID of the group the command was executed on.

.. py:data:: networkId

   The ID of the network the command was executed on.

.. py:data:: currencyName

   The name of the currency.

.. py:data:: scriptCommandPrefix

   The prefix for script commands (usually ``!``).

.. py:data:: metaCommandPrefix

   The prefix for meta commands (usually ``$``).

.. py:data:: extra

   Additional chat service-specific information.
