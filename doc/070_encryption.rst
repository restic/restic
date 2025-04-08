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

##########
Encryption
##########


*"The design might not be perfect, but it’s good. Encryption is a first-class feature,
the implementation looks sane and I guess the deduplication trade-off is worth
it. So… I’m going to use restic for my personal backups.*" `Filippo Valsorda`_

.. _Filippo Valsorda: https://words.filippo.io/restic-cryptography/

**********************
Manage repository keys
**********************

The ``key`` command allows you to set multiple access keys or passwords
per repository. In fact, you can use the ``list``, ``add``, ``remove``, and
``passwd`` (changes a password) sub-commands to manage these keys very precisely:

.. code-block:: console

    $ restic -r /srv/restic-repo key list
    enter password for repository:
     ID        Label               Created
    --------------------------------------------------
    *eb78040b  desktop-backup-key  2015-08-12 13:29:57

    $ restic -r /srv/restic-repo key add --label "laptop-backup-key"
    enter password for repository:
    enter password for new key:
    enter password again:
    saved new key with ID 0123456789abcdef0123456789abcdef01234567

    $ restic -r /srv/restic-repo key list
    enter password for repository:
     ID        Label               Created
    --------------------------------------------------
     5c657874  laptop-backup-key   2015-08-12 13:35:05
    *eb78040b  desktop-backup-key  2015-08-12 13:29:57

Note that the currently used key is indicated by an asterisk (``*``).
