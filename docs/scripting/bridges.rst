Bridges
=======

Bridges are kobun4's way of communicating with chat services.

Discord
-------

.. image:: https://discordapp.com/api/guilds/323659543622057984/embed.png?style=banner2
   :alt: Discord
   :target: https://discord.gg/MNqc3f8

The ``discord`` bridge connects to the `Discord <https://discordapp.com>`_ chat service.

Mappings
~~~~~~~~

 * **Users:** Mapped to Discord users via their numeric ID.

 * **Channels:** Mapped to Discord channels via their numeric ID.

 * **Groups:** Mapped to Discord servers (also known as guilds) via their numeric ID.

 * **Networks:** Only a single network exists, named ``discord``.

Output Formats
~~~~~~~~~~~~~~

The Discord bridge supports the following output formats:

 * ``text``: Specifies a Discord message in plain text. The output will be placed in an embed, and the text message content will contain success information. The embed will be green on success and red on failure.

 * ``rich``: Specifies a Discord message in :ref:`rich <rich>` format.

 * ``discord.embed``: Specifies a `Discord embed <https://discordapp.com/developers/docs/resources/channel#embed-object>`_. The output will be unmarshaled from JSON as an embed and sent.

 * ``discord.embed_multipart``: Specifies a multipart `Discord embed <https://discordapp.com/developers/docs/resources/channel#embed-object>`_ to send. The output will be parsed as a multipart MIME message. The first part of the multipart request will be interpreted as the embed and unmarshaled from JSON. The remaining parts will be considered file attachments.

Deputy Permissions
~~~~~~~~~~~~~~~~~~

The following deputy commands require these permissions to be granted:

 * ``DeleteInputMessage``: Manage Messages
