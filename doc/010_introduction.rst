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

############
Introduction
############

Restic is a fast and secure backup program. In the following sections, we will
present typical workflows, starting with installing, preparing a new
repository, and making the first backup.

Quickstart Guide
****************

To get started with a local repository, first define some environment variables:

.. code-block:: console

    export RESTIC_REPOSITORY=/srv/restic-repo
    export RESTIC_PASSWORD=some-strong-password

Initialize the repository (first time only):

.. code-block:: console

    restic init

Create your first backup:

.. code-block:: console

    restic backup ~/work

You can list all the snapshots you created with:

.. code-block:: console

    restic snapshots

You can restore a backup by noting the snapshot ID you want and running:

.. code-block:: console

    restic restore --target /tmp/restore-work your-snapshot-ID

It is a good idea to periodically check your repository's metadata:

.. code-block:: console

    restic check
    # or full data:
    restic check --read-data

For more details continue reading the next sections.
