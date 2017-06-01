Scripts
=======

Script commands are written by users and can be run by typing ``!command`` (or whatever the script command prefix is instead of ``!``).

To find out information about any script, you can use the ``$cmd`` command:

::

    $cmd command

If you are looking to write your own scripts, the :ref:`scripting documentation <scripting>` contains a reference guide.

Security
--------

Make sure you understand what scripts can and cannot do before running your first script.

**Scripts can:**

 * Collect information about you such as your username and what server and channel you ran the script on.

 * Charge a maximum of the total amount of funds escrowed from your account.

 * :ref:`Bill <billing>` a limited amount of funds from your account for usage.

**Scripts cannot:**

 * Steal your private personal details.

 * Charge you real money.

 * Know what websites you are visiting.

 * Perform anything outside of interacting with your chat user.

Scripts are not screened for safety and they may be malicious! Some precautionary measures are provided (such as :ref:`escrow <escrow>`) but you are responsible for your own safety when running them. Only run scripts from people you trust.

.. _billing:

Billing
-------

Each script you run will be billed according to how long it runs (generally 1 unit of currency per second), up to 5 seconds. If a script runs for too long, you may overdraw the excess amount from your account and your balance will be negative. No payments or bills of any sort may be fulfilled while the balance is overdrawn.
