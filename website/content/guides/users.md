+++
weight = 1
title = "User Guide"
description = "Want to learn more about Kobun? Read this guide!"
+++

Kobun in every server is different, but there's a lot in common from server to server.

## Running commands

You can use `@Kobun help` to get a list of available commands. These commands are made available by your server administrator, and will vary from server to server.

## Private commands

Kobun will run commands in direct message and won't require server administrator permissions. Any script from the [script library](/scripts) is available, e.g. to run a Google search you can message Kobun with `porpoises/google test`.

<div class="alert alert-info">You should not precede commands with a <code>.</code> in private messages. Also since Kobun doesn't know what server you're coming from, it'll only recognize the command names in full from the script library.</div>

## Rate limiting

If you've used Kobun too much, it'll tell you to slow down. This rate limit is global across all servers, but will reset pretty quickly – every 10 seconds, you'll be able to run 1 second of a command, with a limit of 1 second runnable in total.

## Next steps

If you're looking to get Kobun for your own server, [here's how](https://discordapp.com/oauth2/authorize?client_id=315169774688534530&scope=bot&redirect_uri=https%3A%2F%2Fkobun.company%2F_oauthcallback.html&access_type=offline&response_type=token).
