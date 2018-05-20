+++
weight = 3
title = "Scripting Guide"
description = "Want to write your own Kobun commands? Here's a quick rundown."
+++

# Python Scripting Guide

The easiest way to get started using Kobun, is through the online [editor](/editor). You'll need to login with your Discord account if you use the editor for the first time and register an username, make sure that it's all **lowercase** or else it won't work.

After registration you should see a screen similar to this:

![Editor view](/images/guide_editor.png)

Now you're ready to get started.

## Creating your first script

Creating your first script is as easy as picking a name in **lowercase** for your script and clicking <button type="button" class="btn btn-primary">Create</button>. If you script was successfully created, you should see the editor change to the default script as seen here:

![Default script](/images/guide_default_script.png)

You can now already run this script if you copy the command shown in line 3. It will always have the format `@Kobun run` followed by your `username` and `/scriptname`. Run that command in a Discord server that you are the administrator of and somewhere Kobun can read and write messages. You should now see Kobun respond like this:

![Run script](/images/guide_run.png)

*If Kobun doesn't respond, he might not be able to read or write messages, in this case you should check the Discord channel permissions or give Kobun administrative rights.*

As you can see from the response, Kobun will show anything that you've written to `stdout` via functions like `print`, inside an Discord embed along with your mention and showing you if the script ran successfully.

Now try to change that response to anything you like and see if Kobun responds differently, don't forget to **save your script** before calling it again using the <button type="button" class="btn btn-primary">Save</button> button in the top right corner.

## Linking scripts

If you've played around a bit with your script, you'll likely get tired of pasting the long command all the time. That's where linking your script to a custom command comes in handy. To do this however, you first have to set your script to *Unlisted* (private) or *Published* (public) in the top right dropdown menu and save it. Afterwards you can link your script by sending `@Kobun link <command> <username>/<scriptname>`, replacing `<command>` with the command you want and `<username>`,`<scriptname>` with your chosen user name and script name.

Now try linking your first script to `.test` for easy access in the next steps, it should look something like this:

![Link command](/images/guide_link.png)

*If Kobun says script not found, check the spelling and see if it's saved as unlisted or published.*

## How about some input

Making static commands can be fine for some applications but sometimes you need some user input. Getting this input is as easy as calling `sys.stdin.read()`. With this we can create a simple echo script that shows us the user input, just modify your script to look like this:

```python
#!/usr/bin/python3
import sys

inp = sys.stdin.read()
print(inp)
```

If you now send `.test Hello!` Kobun should respond with:

![Echo script](/images/guide_echo.png)

*If Kobun doesn't respond make sure you have linked your script to `.test` as seen in the previous step.*

You always need to separate your chosen command and input with *space*, this space will not be part of the input that gets read.

## Storage options

Kobun provides each user with a limited amount of private storage located at `/mnt/private`, where you can store anything you'd like.

To access this storage outside of your scripts you need to set a password first, by clicking <button type="button" class="btn btn-secondary">Account Info</button> next to your user name in the [editor](/editor) and scroll down to the password fields. After you've set your password you can log onto `https://storage.kobun.company` via any WebDAV client like [WinSCP](https://winscp.net/) using your user name and password.

A good way to store data is inside a sqlite3 database as shown in [rfw/quote](https://kobun.company/scripts/view.html?rfw/quote) or [jacob/pixiepoints](https://kobun.life/scripts/view.html?jacob/pixiepoints).

##  Services

Kobun provides easy access to some Discord services as seen in the [documentation](https://docs.kobun.company/en/latest/scripting/services.html). To make a call to these services you first have to import the library `k4` like this:

```python
import sys
sys.path.insert(0, "/usr/lib/k4")

import k4
```

You can now get the client object with `client = k4.Client()` where the Services are located. Examples for  `client.NetworkInfo` can be found in the script [kornclown/services](https://kobun.company/scripts/view.html?kornclown/services).

## More examples

### Image Link [jacob/example_image_link](https://kobun.life/scripts/view.html?jacob/example_image_link)

**Demonstrates:** Linking an image with comments explaining step-by-step.

### Image Upload [jacob/example_image_upload](https://kobun.life/scripts/view.html?jacob/example_image_upload)

**Demonstrates:** Uploading an image with comments explaining step-by-step.

### Dice Roll [jacob/roll](https://kobun.life/scripts/view.html?jacob/roll) 

**Demonstrates:** Parsing input, random numbers, sending output.

### Cookie [jacob/cookie](https://kobun.life/scripts/view.html?jacob/cookie)

**Demonstrates:** Parsing a tagged user, mentioning a user in reply.

### Pixie Points [jacob/pixiepoints](https://kobun.life/scripts/view.html?jacob/pixiepoints)

**Demonstrates:** Multiple commands, simple point based database for "reputation" system, tagging users, leaderboard.

### Lotto [jacob/lotto](https://kobun.life/scripts/view.html?jacob/lotto) 

**Demonstrates:** Lottery system, implementing admin-only commands.

### Time in [rfw/timein](https://kobun.life/scripts/view.html?rfw/timein)

**Demonstrates:** Sending requests to external services (Google API for time) and loading results .

### Google Image Search [rfw/googleimages](https://kobun.life/scripts/view.html?rfw/googleimages)

**Demonstrates:** Sending requests to external services (Google API for images) and showing the results.

### B Meme [rfw/b](https://kobun.life/scripts/view.html?rfw/b)

**Demonstrates:** Parsing input as tokens (each word), shitty memes.

### Discord Rich Embed [kornclown/richoutput](https://kobun.company/scripts/view.html?kornclown/richoutput)

**Demonstrates:** How to use `Output.SetFormat(format="rich")` for custom Discord rich embeds.
