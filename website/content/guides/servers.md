+++
weight = 2
title = "Server Admin Guide"
description = "Just set up Kobun for your server? Learn how to get started!"
+++

So Kobun's just joined your server. What's next?

## Getting help

Check out `@Kobun help` for some quick information about what's available. Kobun will be a little bit empty to start with – don't worry; you'll be able to get some commands linked in no time!

## Linking commands

The [script library](/scripts) is Kobun's repository of commands. Any script can be _linked_ into Kobun via `@Kobun link`. Only linked scripts can be publicly used on your server.

For instance, you can run `@Kobun link g porpoises/google` to link `.g` to the Google search script `porpoises/google`.

If you don't want a command to be available anymore, you can use `@Kobun unlink` to remove it, e.g. `@Kobun unlink g`.

<div class="alert alert-info">Any server administrator will be able to run unlinked commands. For example, if you want to run <code>porpoises/google</code> without making it available for everyone, you can use <code>.porpoises/google</code> directly.</div>

## Managing permissions

By default, the server founder and anyone with administrator permissions will be able to manage command linking. If you want to grant command management permissions to someone without granting such a broad range of permissions, create and grant a role named `Kobun Administrators` (it must be named **exactly** that).

## Command prefixes

The default command prefix for Kobun is `.`, and can be changed using `@Kobun prefix`. For instance, you can use `@kobun prefix k.` to change the command prefix in your server to `k.`.

## Writing commands

Check out the [scripting guide](/guides/scripting) for information.
