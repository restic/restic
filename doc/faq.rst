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
the environment variables ``RESTIC_PASSWORD`` or ``RESTIC_PASSWORD_FILE``
environment variables. A discussion is in progress over implementing unattended
backups happens in :issue:`533`.

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
