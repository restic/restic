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

.. _Filippo Valsorda: https://blog.filippo.io/restic-cryptography/

**********************
Manage repository keys
**********************

The ``key`` command allows you to set multiple access keys or passwords
per repository. In fact, you can use the ``list``, ``add``, ``remove``, and
``passwd`` (changes a password) sub-commands to manage these keys very precisely:

.. code-block:: console

    $ restic -r /srv/restic-repo key list
    enter password for repository:
     ID          User        Host        Created
    ----------------------------------------------------------------------
    *eb78040b    username    kasimir   2015-08-12 13:29:57

    $ restic -r /srv/restic-repo key add
    enter password for repository:
    enter password for new key:
    enter password again:
    saved new key as <Key of username@kasimir, created on 2015-08-12 13:35:05.316831933 +0200 CEST>

    $ restic -r /srv/restic-repo key list
    enter password for repository:
     ID          User        Host        Created
    ----------------------------------------------------------------------
     5c657874    username    kasimir   2015-08-12 13:35:05
    *eb78040b    username    kasimir   2015-08-12 13:29:57

**************************
Using master key from file
**************************

The flag ``--masterkeyfile`` allows you to directly use a file for the master key. Then no key needs to be stored
in the repository. Make sure your masterkey cannot be accessed by unauthorized persons and that you  have a backup
of you masterkey file ready (e.g. print-out)!


.. code-block:: console

    $ restic -r /srv/restic-repo --masterkeyfile master.key init
    created restic repository f855d38126 at /tmp/repo

    Please note that you need the masterkey tmp.master to access the repository
    Losing your masterkey file means that your data is irrecoverably lost.

    $ restic -r /srv/restic-repo --masterkeyfile master.key key list
    repository f855d381 opened successfully
    ID  User  Host  Created
    ------------------------
    ------------------------

If you want to export the master key from an existing repository, you can use the ``cat masterkey`` command:

.. code-block:: console

    $ restic -r /srv/restic-repo init
    enter password for new repository: 
    enter password again: 
    created restic repository a8e7f96567 at /srv/restic-repo

    Please note that knowledge of your password is required to access
    the repository. Losing your password means that your data is
    irrecoverably lost.
    $ restic -r  /srv/restic-repo --quiet cat masterkey > key.master 
    enter password for repository:
    $ chmod 400 key.master
    $ restic -r /srv/restic-repo --masterkeyfile key.master key list
    repository a8e7f965 opened successfully
    ID        User        Host      Created
    ---------------------------------------------------
    335ca24b  username    kasimir   2015-08-12 13:35:05
    ---------------------------------------------------
    $ restic -r /srv/restic-repo --masterkeyfile key.master key remove 335ca24b
    repository a8e7f965 opened successfully
    removed key 335ca24bc44d561f60b700bc2b52c1850dc1a97094d755dd7c1c185b91111d9b
