FAQ
===

This is the list of Frequently Asked Questions for restic.

``restic check`` reports packs that aren't referenced in any index, is my repository broken?
--------------------------------------------------------------------------------------------

When ``restic check`` reports that there are pack files in the
repository that are not referenced in any index, that's (in contrast to
what restic reports at the moment) not a source for concern. The output
looks like this:

::

    $ restic check
    Create exclusive lock for repository
    Load indexes
    Check all packs
    pack 819a9a52e4f51230afa89aefbf90df37fb70996337ae57e6f7a822959206a85e: not referenced in any index
    pack de299e69fb075354a3775b6b045d152387201f1cdc229c31d1caa34c3b340141: not referenced in any index
    Check snapshots, trees and blobs
    Fatal: repository contains errors

The message means that there is more data stored in the repo than
strictly necessary. With high probability this is duplicate data. In
order to clean it up, the command ``restic prune`` can be used. The
cause of this bug is not yet known.

How can I specify encryption passwords automatically?
-----------------------------------------------------

When you run ``restic backup``, you need to enter the passphrase on
the console. This is not very convenient for automated backups, so you
can also provide the password through the ``--password-file`` option, or one of
the environment variables ``RESTIC_PASSWORD`` or ``RESTIC_PASSWORD_FILE``.
A discussion is in progress over implementing unattended backups happens in
:issue:`533`.

.. important:: Be careful how you set the environment; using the env
               command, a `system()` call or using inline shell
               scripts (e.g. `RESTIC_PASSWORD=password restic ...`)
               might expose the credentials in the process list
               directly and they will be readable to all users on a
               system. Using export in a shell script file should be
               safe, however, as the environment of a process is
               `accessible only to that user`_. Please make sure that
               the permissions on the files where the password is
               eventually stored are safe (e.g. `0600` and owned by
               root).

.. _accessible only to that user: https://security.stackexchange.com/questions/14000/environment-variable-accessibility-in-linux/14009#14009

How to prioritize restic's IO and CPU time
------------------------------------------

If you'd like to change the **IO priority** of restic, run it in the following way

::

$ ionice -c2 -n0 ./restic -r /media/your/backup/ backup /home

This runs ``restic`` in the so-called best *effort class* (``-c2``),
with the highest possible priority (``-n0``).

Take a look at the `ionice manpage`_ to learn about the other classes.

.. _ionice manpage: https://linux.die.net/man/1/ionice


To change the **CPU scheduling priority** to a higher-than-standard
value, use would run:

::

$ nice --10 ./restic -r /media/your/backup/ backup /home

Again, the `nice manpage`_ has more information.

.. _nice manpage: https://linux.die.net/man/1/nice

You can also **combine IO and CPU scheduling priority**:

::

$ ionice -c2 nice -n19 ./restic -r /media/gour/backup/ backup /home

This example puts restic in the IO class 2 (best effort) and tells the CPU
scheduling algorithm to give it the least favorable niceness (19).

The above example makes sure that the system the backup runs on
is not slowed down, which is particularly useful for servers.

Creating new repo on a Synology NAS via sftp fails
--------------------------------------------------

Sometimes creating a new restic repository on a Synology NAS via sftp fails
with an error similar to the following:

::

    $ restic init -r sftp:user@nas:/volume1/restic-repo init
    create backend at sftp:user@nas:/volume1/restic-repo/ failed:
        mkdirAll(/volume1/restic-repo/index): unable to create directories: [...]

Although you can log into the NAS via SSH and see that the directory structure
is there.

The reason for this behavior is that apparently Synology NAS expose a different
directory structure via sftp, so the path that needs to be specified is
different than the directory structure on the device and maybe even as exposed
via other protocols.


Try removing the /volume1 prefix in your paths. If this does not work, use sftp
and ls to explore the SFTP file system hierarchy on your NAS.

The following may work:

::

    $ restic init -r sftp:user@nas:/restic-repo init
