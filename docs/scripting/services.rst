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

Accounts
--------

The accounts service provides methods to look up accounts.

.. py:function:: Accounts.Lookup(alias : string) -> string

   Looks up a user's account handle from their alias.

   :param alias: The alias to look up. The alias is generally of the format ``<bridge name>/<user ID>``.
   :return: The user's account handle.

Bridge
------

The bridge service provides scripts with the ability to interact with the chat service they're running on.

.. py:function:: Bridge.GetUserInfo(id : string) -> {name: string, extra: object}

   Looks up a user's information by their user ID.

   :param id: The ID to look up.
   :return: Information about the user. ``extra`` contains additional chat service-specific information.

.. py:function:: Bridge.GetChannelInfo(id : string) -> {name: string, isOneOnOne: boolean, extra: object}

   Looks up a channel's information by its channel ID.

   :param id: The ID to look up.
   :return: Information about the channel. ``isOneOnOne`` is true if and only if the channel is a private channel with the bot. ``extra`` contains additional chat service-specific information.

.. py:function:: Bridge.GetServerInfo(id : string) -> {name: string, extra: object}

   Looks up a server's information by its server ID.

   :param id: The ID to look up.
   :return: Information about the server. ``extra`` contains additional chat service-specific information.

Money
-----

The money service provides scripts to charge and pay the user executing the script.

.. py:function:: Money.Charge(targetAccountHandle: string, amount: number)

   Charges a user and deposits their money into ``targetAccountHandle``.

   .. note:: Charges can only be made from :ref:`escrowed <escrow>` funds, and will be reported to the user directly after the script finishes.

   :param targetAccountHandle: The account to deposit the charge into.
   :param amount: The amount to charge.

.. py:function:: Money.Pay(targetAccountHandle: string, amount: number)

   Pays a user, depositing the money into ``targetAccountHandle``.

   :param targetAccountHandle: The account to deposit the payment into.
   :param amount: The amount to pay.

.. py:function:: Money.Transfer(sourceAccountHandle: string, sourceAccountKey: string, targetAccountHandle: string, amount: number)

   Initiates a direct transfer of money from the source account.

   .. warning:: Transfers are **direct** and will bypass the escrow limit. Withdrawals done via transfer will also not be reported.

   :param sourceAccountHandle: The account to withdraw from.
   :param sourceAccountKey: The key of the account to withdraw from.
   :param targetAccountHandle: The account to deposit into.
   :param amount: The amount to transfer.

.. py:function:: Money.GetBalance(accountHandle: string) -> number

   Gets the balance of an account.

   :param accountHandle: The account to get the balance of.
   :return: The account's balance.

Output
------

The output service allows scripts to set out-of-band metadata on the output of scripts.

.. py:function:: Output.SetFormat(format: string)

   Sets the output format of the script. The default is ``text``, which will be interpreted as simple text output. Other formats are dependent on the chat service the script is being executed on.

   :param format: The output format to use.
