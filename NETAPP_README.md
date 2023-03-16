#Netapp Readme
# Overview
This fork has changes that make restic work with NetApp ONTAP and Storage Grid.
Its the same as the netapp/restic, just moved to polaris/restic

This repo uses main for its default branch and will update with master from restic/restic as we need.

The intent is to make these changes open source and let people enjoy them.  

NetApp does not imply or guarentee any support, but we are happy to help if we can.  File an issue or email the contributors if you need help

# License Notice
The license for Restic is 2 Clause BSD.  We will maintain that license but deep open source scans done by NetApp show some golang modules
include GPLv2 and LGPL v3 code.  Please keep that in mind when you use this software.  Again, this is in base Restic, nothing we have added.


# Development
We shall NEVER push to master.  Master is reserved for the upstream

The master branch for this fork is netapp-main.  
We pull down the changes and then merge the most recent released tag to netapp-main


# Merging of upstream changes example workflow

1) git checkout -b  merge-work-0.15.1-to-netapp
2) git merge v0.15.1 
3) fix conflicts related to these known patches:
    - /.github/workflows/netapp-cicd.yml
    - /restic/cmd_forget.go , added option `deleteEmptyRepo`
    - /cmd/restic/cmd_init.go ,  modified `s.Init` and added `wasCreated`
    - /cmd/restic/cmd_version.go , `netappversion`
    - /cmd/restic/global.go , `var netappversion =` , `ontap.ProtocolScheme` 
    - /b/go.mod , replace section , keep latest library always
    - /internal/backend/azure/azure.go `timeout` , `func init()` , `restic.KeysFile`  added 
    - /internal/backend/gs/gs.go  `restic.KeysFile`  added
    - /internal/backend/http_transport.go , `FORCE_CERT_VALIDATION` added
    - /internal/backend/s3/s3.go `removeKeys` modified ,  `restic.KeysFile` added 
    - /internal/repository/repository.go modifies `(r *Repository) Init` adds bool return
    - /internal/restic/file.go , adds `KeysFile`
    - /internal/errors/errors.go , keep using `Cause` (upstream dropped it, but we use it still)
4) accept incoming changes: select files in vscode, right click, accept all incoming
5) try to compile and fix as needed (e.g. duplicated code sections are common!)
    - To see what is different between upstream and my new branch: `git diff upstream/v0.15.1 merge-work-0.15.1-to-netapp`
6) update `netappversion` and file `NETAPPVERSION`