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

Design Principles
*****************

Restic is a program that does backups right and was designed with the following principles in mind:

- **Easy:** Doing backups should be a frictionless process, otherwise you might be tempted to skip it. Restic should be easy to configure and use, so that, in the event of a data loss, you can just restore it. Likewise, restoring data should not be complicated.
- **Fast:** Backing up your data with Restic should only be limited by your network or hard disk bandwidth so that you can backup your files every day. Nobody does backups if it takes too much time. Restoring backups should only transfer data that is needed for the files that are to be restored, so that this process is also fast.
- Verifiable: Much more important than backup is restore, so Restic enables you to easily verify that all data can be restored.
- **Secure:** Restic uses cryptography to guarantee confidentiality and integrity of your data. The location the backup data is stored is assumed not to be a trusted environment (e.g. a shared space where others like system administrators are able to access your backups). Restic is built to secure your data against such attackers.
- **Efficient:** With the growth of data, additional snapshots should only take the storage of the actual increment. Even more, duplicate data should be de-duplicated before it is actually written to the storage back end to save precious backup space.

Backends supported
******************

Saving a backup on the same machine is nice but not a real backup strategy. Therefore, Restic supports the following backends for storing backups natively:

- Local directory
- sftp server (via SSH)
- HTTP REST server (protocol rest-server)
- AWS S3 (either from Amazon or using the Minio server)
- OpenStack Swift
- BackBlaze B2
- Microsoft Azure Blob Storage
- Google Cloud Storage
- And many other services via the rclone Backend
