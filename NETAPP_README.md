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


