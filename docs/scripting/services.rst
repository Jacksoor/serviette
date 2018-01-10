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

   Sets the :ref:`output format <output-formats>` of the script. The default is ``text``, which will be interpreted as simple text output. Other formats are dependent on the chat service the script is being executed on.

   :param format: The output format to use.

.. py:function:: Output.SetPrivate(private: boolean)

   Sets the output of the script to be sent to a private message.

   :param private: Whether or not the output should be sent via a private message.

.. py:function:: Output.SetExpires(expires: boolean)

   Sets whether or not the message should expire.

   :param expire: Whether or not the message should expire.

.. _output-formats:

Formats
~~~~~~~

text
++++

Plain text format.

.. _rich:

rich
++++

Rich content format. Must be in the JSON with the following format:

.. code-block:: javascript

    {
        "fallback": "...",       // (required) fallback plain text for non-rich content bridges
        "color": 0,              // (optional) color as an 24-bit integer in RGB order
        "author": "...",         // (optional) author of the content
        "authorLink": "...",     // (optional) link to the author
        "authorIconURL: "...",   // (optional) URL to an icon representing the author
        "title": "...",          // (optional) title of the content
        "titleLink": "...",      // (optional) link for the title
        "text: "... ",           // (optional) description of the content
        "fields": [              // (optional) list of fields describing the content
            {
                "name": "...",   // (required) name of the field
                "value": "...",  // (required) content of the field
                "inline": false, // (optional) attempt to save save by packing fields horizontally when possible
            },
            ...
        ],
        "imageURL": "...",       // (optional) URL to an image representing the content
        "thumbnailURL": "...",   // (optional) URL to a thumbnail representing the content
        "footer": "...",         // (optional) text to place in the footer
        "footerIconURL": "...",  // (optional) icon to show next to the footer
        "timestamp": 0           // (optional) UNIX timestamp of when the content was produced
    }

.. _deputy:

Deputy
------

The deputy service allows Kobun to perform certain restricted administrative tasks on behalf of the command issuer.

These commands are those that the command issuer would have been able to take themselves.

Please refer to the documentation for your :ref:`bridge <bridges>` to determine how to grant the correct permissions for these features.

.. warning:: Please make sure you understand the security implications of granting Kobun administrative permissions! The developers of Kobun are not liable for any damages or losses incurred by enabling these features!

.. py:function:: Deputy.DeleteInputMessage()

   Deletes the input message used to trigger the command, if supported.

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
