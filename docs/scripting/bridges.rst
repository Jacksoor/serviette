Bridges
=======

Bridges are kobun4's way of communicating with chat services.

Discord
-------

The `Discord <discord>`_ bridge (``discord``) connects to Discord.

.. _discord: https://discordapp.com

Output Formats
~~~~~~~~~~~~~~

The Discord bridge supports the following output formats:

 * **``text``:** Specifies a Discord message in plain text. The output will be placed in an embed, and the text message content will contain success and billing information. The embed will be green on success and red on failure.

 * **``discord.embed``:** Specifies a `Discord embed <discordembed>`_. The output will be unmarshaled from JSON as an embed and sent.

 * **``discord.embed_multipart``:** Specifies a multipart `Discord message <discordembed>`_ to send. The output will be parsed as a multipart MIME message. The first part of the multipart request will be interpreted as the embed and unmarshaled from JSON. The remaining parts will be considered file attachments.

.. _discordcreatemessage: https://discordapp.com/developers/docs/resources/channel#create-message
