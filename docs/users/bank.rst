The Bank
========

Money makes the kobun4 world go around, and in kobun4 it's all kept in the bank.

Generally, 10 unit of currency is equivalent to 1 unit of processing time (generally 1 unit of currency = 1 second of processing time).

.. _accounts:

Accounts
--------

An account is identified by two things – its *handle* and its *key*. Users can make transfers into your account knowing only its *handle*, but only you or another holder of your *key* can make transfers out of your account.

Basic Commands
--------------

Income
~~~~~~

Income is received on a per-character basis – each character sent by you will pay out a certain amount (generally 1 character = 1 unit of currency).

Balance
~~~~~~~

::

    $balance [mention/handle]

You can look up your balance simply by using ``$balance``. You can look up anyone else's balance by following the command with either their mention or their account handle.

::

    $account [mention/handle]

To look up your account handle or anyone else's, you can substitute ``$balance`` for ``$account``.

.. _escrow:

Escrow
~~~~~~

::

    $escrow amount command [arg]


Funds may be placed in *escrow* for a script via ``$escrow``. Scripts may only charge up to the escrowed limit, i.e. you will not spend more money than the amount escrowed for the command.

If a command with escrowed funds manages to successfully charge your account, the charge will shown as part of the result.

Payments
~~~~~~~~

::

    $pay mention/handle amount

Payments are free and can be sent to anyone by either their mention or their account handle. You cannot pay someone more than you have in your balance.

Advanced Commands
-----------------

Key
~~~

::

    $key

**This command will only be responded to in one-on-one conversations.** You can look up your account's key, to log into :ref:`scripting <scripting>` facilities.

New Orphan
~~~~~~~~~~

::

    $neworphan

**This command will only be responded to in one-on-one conversations.** You can create *orphan* accounts – accounts that are not associated with any user. You may choose to use orphan accounts to sequester your funds from your primary day-to-day account. The handle and key will be sent to you only once, so keep them safe somewhere!

Transfer
~~~~~~~~

::

    $transfer sourcemention/sourcehandle sourcekey targetmention/targethandle amount

**This command will only be responded to in one-on-one conversations.** This initiates a **direct** transfer from another account to your account. You must provide the key of the account you are transferring from, as that account's consent will **not be required**.
