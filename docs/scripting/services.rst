.. _services:

Services
========

kobun4 exposes a set of *services* to scripts. These services allow scripts to communicate out-of-band with kobun4 to access facilities that extend beyond simple input/output.

Services are accessible via a socket connection on file descriptor 3. Requests are sent JSON-marshaled with no line breaks permitted, terminated with a newline:

.. code-block:: javascript

  {"id": /* (number) sequence number */, "method": /* (string) method name */, "params": [/* (object) request body */]}

Requests are similarly received JSON-marshaled on a single line, terminated with a newline:

.. code-block:: javascript

  {"id": /* (number) sequence number */, "error": /* (string?) error, null if no error */, "result": /* (object?) result, null if error */}

The response's ID must match the request's ID, otherwise the response must not be considered the response to a sent request.

The following sections describe the services available via this RPC interface.

NetworkInfo
-----------

The network information service provides scripts with the ability to interact with the network they're running on.

.. py:function:: NetworkInfo.GetUserInfo(id: string) -> {name: string, extra: object}

   Looks up a user's information by their user ID.

   :param id: The ID to look up.
   :return: Information about the user. ``extra`` contains additional network-specific information.

.. py:function:: NetworkInfo.GetChannelInfo(id: string) -> {name: string, isOneOnOne: boolean, extra: object}

   Looks up a channel's information by its channel ID.

   :param id: The ID to look up.
   :return: Information about the channel. ``isOneOnOne`` is true if and only if the channel is a private channel with the bot. ``extra`` contains additional network-specific information.

.. py:function:: NetworkInfo.GetGroupInfo(id: string) -> {name: string, extra: object}

   Looks up a group's information by its group ID.

   :param id: The ID to look up.
   :return: Information about the group. ``extra`` contains additional network-specific information.

.. py:function:: NetworkInfo.GetChannelMemberInfo(channelId: string, userId: string) -> {name: string, roles: string[], extra: object}

   Looks up a channel member's information by a channel ID and their user ID.

   :param channelId: The ID of the channel the user is a member of.
   :param userId: The user ID of the member to look up.
   :return: Information about the channel member. ``name`` may contain their channel-specific username – if channel-specific usernames do not exist, their regular username will be returned. ``extra`` contains additional network-specific information.

.. py:function:: NetworkInfo.GetGroupMemberInfo(groupId: string, userId: string) -> {name: string, roles: string[], extra: object}

   Looks up a group member's information by a group ID and their user ID.

   :param groupId: The ID of the group the user is a member of.
   :param userId: The user ID of the member to look up.
   :return: Information about the group member. ``name`` may contain their group-specific username – if group-specific usernames do not exist, their regular username will be returned. ``extra`` contains additional network-specific information.

Output
------

The output service allows scripts to set out-of-band metadata on the output of scripts.

.. py:function:: Output.SetFormat(format: string)

   Sets the output format of the script. The default is ``text``, which will be interpreted as simple text output. Other formats are dependent on the chat service the script is being executed on.

   :param format: The output format to use.

.. py:function:: Output.SetPrivate(private: boolean)

   Sets the output of the script to be sent to a private message.

   :param private: Whether or not the output should be sent via a private message.

Messaging
---------

.. note:: Permissions to use the messaging service must be granted explicitly by an operator of Kobun.

The messaging service allows scripts to message users or channels out-of-band.

.. py:function:: Messaging.MessageUser(id: string, format: string, content: string)

   Sends a direct message to a user.

   :param id: The user ID to send the message to.
   :param format: The output format to send the message with.
   :param content: The content to send.

.. py:function:: Messaging.MessageChannel(id: string, format: string, content: string)

   Sends a message to a channel.

   :param id: The channel ID to send the message to.
   :param format: The output format to send the message with.
   :param content: The content to send.
