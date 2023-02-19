FAQ
===

This is the list of Frequently Asked Questions for restic.

Will restic resume an interrupted backup?
-----------------------------------------

Yes, restic will resume interrupted backups when they are re-run.

When backing up, restic periodically writes index files to keep a record of
the uploaded data. Even if there's no snapshot created in the end (due to the
backup being interrupted), these indexes are stored in the repository for the
data that has been uploaded so far. Next time restic runs, it is then able to
find the uploaded data through these indexes, and thereby reference it again
without having to upload it a second time. This effectively makes it continue
from where it saved the last index, which should be up to a few minutes ago.

It does not matter if the backup was interrupted by the user or if it was due
to unforeseen circumstances such as connectivity issues, power loss, etc.
Simply re-run the backup again and restic should only upload what it needs to
in order to complete the interrupted backup and create a snapshot.

Note however that during the initial backup run and any re-tries, until there
has been a first snapshot created for the backup set (list of files and
directories to be backed up), restic will need to re-scan the files on disk as
there is no parent snapshot to compare the filesystem with to determine which
files have changed. This process should however be far quicker than the
uploading, and it's normal to see restic scan the files again when re-running
the backup.

``restic check`` reports packs that aren't referenced in any index, is my repository broken?
--------------------------------------------------------------------------------------------

When ``restic check`` reports that there are pack files in the
repository that are not referenced in any index, that's (in contrast to
what restic reports at the moment) not a source for concern. The output
looks like this:

::

    $ restic check
    create exclusive lock for repository
    load indexes
    check all packs
    pack 819a9a52e4f51230afa89aefbf90df37fb70996337ae57e6f7a822959206a85e: not referenced in any index
    pack de299e69fb075354a3775b6b045d152387201f1cdc229c31d1caa34c3b340141: not referenced in any index
    2 additional files were found in the repo, which likely contain duplicate data.
    You can run `restic prune` to correct this.
    check snapshots, trees and blobs
    [0:00] 100.00%  16 / 16 snapshots
    no errors were found

The message means that there is more data stored in the repository than
strictly necessary. This is uncritical. With high probability this is duplicate data
caused by an interrupted backup run or upload operation. In
order to clean it up, the command ``restic prune`` can be used.

I ran a ``restic`` command but it is not working as intended, what do I do now?
-------------------------------------------------------------------------------

If you are running a restic command and it is not working as you hoped it would,
there is an easy way of checking how your shell interpreted the command you are trying to run.

Here is an example of a mistake in a backup command that results in the command not working as expected.
A user wants to run the following ``restic backup`` command

::

$ restic backup --exclude "~/documents" ~

.. important:: This command contains an intentional user error described in this paragraph.

This command will result in a complete backup of the current logged in user's home directory and it won't exclude the folder ``~/documents/`` - which is not what the user wanted to achieve.
The problem is how the path to ``~/documents`` is passed to restic.

In order to spot an issue like this, you can make use of the following ruby command preceding your restic command.

::

    $ ruby -e 'puts ARGV.inspect' restic backup --exclude "~/documents" ~
    ["restic", "backup", "--exclude", "~/documents", "/home/john"]

As you can see, the command outputs every argument you have passed to the shell. This is what restic sees when you run your command.
The error here is that the tilde ``~`` in ``"~/documents"`` didn't get expanded as it is quoted.

::

    $ echo ~/documents
    /home/john/documents

    $ echo "~/documents"
    ~/document

    $ echo "$HOME/documents"
    /home/john/documents

Restic handles globbing and expansion in the following ways:

-  Globbing is only expanded for lines read via ``--files-from``
-  Environment variables are not expanded in the file read via ``--files-from``
-  ``*`` is expanded for paths read via ``--files-from``
-  e.g. For backup targets given to restic as arguments on the shell, neither glob expansion nor shell variable replacement is done. If restic is called as ``restic backup '*' '$HOME'``, it will try to backup the literal file(s)/dir(s) ``*`` and ``$HOME``
-  Double-asterisk ``**`` only works in exclude patterns as this is a custom extension built into restic; the shell must not expand it


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

Creating new repository on a Synology NAS via sftp fails
--------------------------------------------------------

For using restic with a Synology NAS via sftp, please make sure that the
specified path is absolute, it must start with a slash (``/``).

Sometimes creating a new restic repository on a Synology NAS via sftp fails
with an error similar to the following:

::

    $ restic -r sftp:user@nas:/volume1/restic-repo init
    create backend at sftp:user@nas:/volume1/restic-repo/ failed:
        mkdirAll(/volume1/restic-repo/index): unable to create directories: [...]

Although you can log into the NAS via SSH and see that the directory structure
is there.

The reason for this behavior is that apparently Synology NAS expose a different
directory structure via sftp, so the path that needs to be specified is
different than the directory structure on the device and maybe even as exposed
via other protocols.


Try removing the ``/volume1`` prefix in your paths. If this does not work, use
sftp and ls to explore the SFTP file system hierarchy on your NAS.

The following may work:

::

    $ restic -r sftp:user@nas:/restic-repo init

Why does restic perform so poorly on Windows?
---------------------------------------------

In some cases the real-time protection of antivirus software can interfere with
restic's operations. If you are experiencing bad performance you can try to
temporarily disable your antivirus software to find out if it is the cause for
your performance problems. If you are certain that the antivirus software is
the cause for this and you want to gain maximum performance, you have to add
the restic binary to an exclusions list within the antivirus software.

How do I choose a strong password?
----------------------------------

Length is the single most important component in password strength.  That doesn't
mean that other components such as complexity and entropy (or randomness) are not
important to consider.  A strong password includes Alphabetical, Numerical and
Special characters. For example, ``nk3E9Rr26md6GGySyyWMrfakw8Jck4$&vVY6`` would
be a very strong password, if not for being in this documentation.

There are plenty of tools out there, such as OpenSSL, pwgen or KeePass that can
generate a sufficiently complex, random and long password.

Restic backup command fails to find a valid file in Windows
-----------------------------------------------------------

If the name of a file in Windows contains an invalid character, Restic will not be
able to read the file. To solve this issue, consider renaming the particular file.
