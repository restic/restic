Foreword
========

WARNING: At the moment, consider khepri as alpha quality software, it is not
yet finished. Do not use it for real data yet.

Khepri
======

Khepri is a program that does backups right. It tries to fulfill the
following design goals:

 * Easy: Every obstacle that is between you and a working
   backup solution, especially regarding complex or otherwise hard to use
   software. Khepri should be easy to configure and use, so that in the
   unlikely event of a catastrophic data loss, you are safe. Likewise,
   restoring data should not be complicated.

 * Fast: Backing up your data with khepri should only be
   limited by your network or harddisk bandwidth. Ideally, you should backup
   your files every day.  Nobody does backups if it is so slow it takes too
   much time. Furthermore, restoring backups should only transfer data that
   is needed for the files that are to be restored, so that restoring is
   also fast.

 * Verifiable: Much more important than backup is restore, so khepri makes it
   easy to verify regularly that all data can be restored.

 * Secure: Khepri uses cryptography to guarantee integrity and
   confidentiality of your data. The location where the backups is assumed
   to be a shared space where others (e.g. a system administrator) is able
   to access your backups. Khepri is built to secure your data against such
   attackers.

License
=======

Khepri is licensed under "BSD 2-Clause License". You can find the complete text
in the file `LICENSE`.
