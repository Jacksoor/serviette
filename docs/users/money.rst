Money
=====

Money makes the kobun4 world go around.

Fundamentally, 1 unit of currency is equivalent to 1 unit of processing time (generally 1 unit of currency = 1 second of processing time).

Accounts
--------

An account is identified by two things – its *handle* and its *key*. Users can make transfers into your account knowing only its *handle*, but only you or another holder of your *key* can make transfers out of your account.

.. _billing:

Billing
-------

Each command you run will be billed according to how long it runs (generally 1 unit of currency per second), up to 5 seconds. If a command runs for too long, you may overdraw the excess amount from your account and your balance will be negative. No payments or bills of any sort may be fulfilled while the balance is overdrawn.

Earning
-------

.. _escrow:

Escrow
------

::

    $escrow amount command [arg]


Funds may be placed in *escrow* for a script via ``$escrow``. Scripts may only charge up to the escrowed limit, i.e. you will not spend more money than the amount escrowed for the command.
