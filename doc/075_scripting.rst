..
  Normally, there are no heading levels assigned to certain characters as the structure is
  determined from the succession of headings. However, this convention is used in Python’s
  Style Guide for documenting which you may follow:

  # with overline, for parts
  * for chapters
  = for sections
  - for subsections
  ^ for subsubsections
  " for paragraphs

#########################
Scripting
#########################

This is a list of how certain tasks may be accomplished when you use
restic via scripts.

Check if a repository is already initialized
********************************************

You may find a need to check if a repository is already initialized,
perhaps to prevent your script from initializing a repository multiple
times. The command ``snapshots`` may be used for this purpose:

.. code-block:: console

    $ restic -r /srv/restic-repo snapshots
    Fatal: unable to open config file: Stat: stat /srv/restic-repo/config: no such file or directory
    Is there a repository at the following location?
    /srv/restic-repo

If a repository does not exist, restic will return a non-zero exit code
and print an error message. Note that restic will also return a non-zero
exit code if a different error is encountered (e.g.: incorrect password
to ``snapshots``) and it may print a different error message. If there
are no errors, restic will return a zero exit code and print all the
snapshots.
