Bridges
=======

Bridges are kobun4's way of communicating with chat services.

Discord
-------

.. image:: https://discordapp.com/api/guilds/315164870997835777/embed.png
   :alt: merc-devel
   :target: https://discord.gg/bRCvFy9

The ``discord`` bridge connects to the `Discord <discordweb>`_ chat service.

.. _discordweb: https://discordapp.com

Mappings
~~~~~~~~

 * **Users:** Mapped to Discord users via their numeric ID.

 * **Channels:** Mapped to Discord channels via their numeric ID.

 * **Groups:** Mapped to Discord servers (also known as guilds) via their numeric ID.

 * **Networks:** Only a single network exists, named ``discord``.

Output Formats
~~~~~~~~~~~~~~

The Discord bridge supports the following output formats:

 * ``text``: Specifies a Discord message in plain text. The output will be placed in an embed, and the text message content will contain success and billing information. The embed will be green on success and red on failure.

 * ``discord.embed``: Specifies a `Discord embed <discordembed>`_. The output will be unmarshaled from JSON as an embed and sent.

 * ``discord.embed_multipart``: Specifies a multipart `Discord embed <discordembed>`_ to send. The output will be parsed as a multipart MIME message. The first part of the multipart request will be interpreted as the embed and unmarshaled from JSON. The remaining parts will be considered file attachments.

.. _discordembed: https://discordapp.com/developers/docs/resources/channel#embed-object
