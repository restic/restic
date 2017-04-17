# Setting up Restic with Amazon S3

## Preface

This tutorial will show you how to use Restic with AWS S3. It will show you how
to navigate the AWS web interface, create an S3 bucket, create a user with
access to only this bucket, and finally how to connect Restic to this bucket.

## Prerequisites

You should already have a `restic` binary available on your system that you can
run. Furthermore, you should also have an account with
[AWS](https://aws.amazon.com/). You will likely need to provide credit card
details for billing purposes, even if you use their
[free-tier](https://aws.amazon.com/free/). 


## Logging into AWS

Point your browser to
https://console.aws.amazon.com
and log in using your AWS account. You will be presented with the AWS homepage:
![AWS Homepage](img/s3/01_aws_start.png?raw=true)

By using the "Services" button in the upper left corder, a menu of all services
provided by AWS can be opened:
![AWS Services Menu](img/s3/02_aws_menu.png?raw=true)

For this tutorial, the Simple Storage Service (S3), as well as Identity and
Access Management (IAM) are relevant.


## Creating the bucket

First, a bucket to store your backups in must be created. Using the "Services"
menu, navigate to S3. In case you already have some S3 buckets, you will see a
list of them here:
![List of S3 Buckets](img/s3/03_buckets_list_before.png?raw=true)

Click the "Create bucket" button and choose a name and region for your new
bucket. For the purpose of this tutorial, the bucket will be named
`restic-demo` and reside in Frankfurk. Because the bucket namespace is shared among all AWS users, the
name `restic-demo` may not be available to you. Be creative and choose a unique
bucket name.
![Creating a Bucket](img/s3/04_bucket_create_start.png?raw=true)

It is not necessary to configure any special properties or permissions of the
bucket just yet. Therefore, just finish the wizard without making any further
changes:
![Reviewing Bucket Creation](img/s3/05_bucket_create_review.png?raw=true)

The newly created `restic-demo` bucket will no appear on the list of S3
buckets:
![List With New Bucket](img/s3/06_buckets_list_after.png?raw=true)


## Creating a user

Use the "Services" menu of the AWS web interface to navigate to IAM. This will
bring you to the IAM homepage. To create a new user, click on the "Users" menu
entry on the left:
![IAM Homepage](img/s3/07_iam_start.png?raw=true)

In case you already have set-up users with IAM before, you will see a list of
them here. Use the "Add user" button at the top to create a new user:
![IAM User List](img/s3/08_user_list.png?raw=true)

For this tutorial, the new user will be named `restic-demo-user`. Feel free to
choose your own name that best fits your needs. This user will only ever access
AWS through the `restic` program and not through the web interface. Therefore,
"Programmatic access" is selected for "Access type":
![Choosing Username And Access Type](img/s3/09_user_name.png?raw=true)

During the next step, permissions can be assigned to the new user. To use this
user with Restic, it only needs access to the `restic-demo` bucket. Select
"Attach exiting policies directly", which will bring up a list of pre-defined
policies below. Afterwards, click the "Create policy" button to create a custom
policy:
![Assigning a Policy](img/s3/10_user_pre_policy.png?raw=true)

A new browser window or tab will open with the policy wizard. In Amazon IAM,
policies are defined as JSON documents. For this tutorial, the "Policy
Generator" will be used to generate a policy file using a web interface:
![Creating a New Policy](img/s3/11_policy_start.png?raw=true)

After invoking the policy generator, you will be presented with a user
interface to generate individual permission statements. For Restic to work, two
such statements must be created. The first statement is set up as follows:

```
Effect: Allow
Service: S3
Actions: DeleteObject, GetObject, PutObject
Resource: arn:aws:s3:::restic-demo/*
```

This statement allows Restic to create, read and delete objects inside the S3
bucket named `restic-demo`. Adjust the bucket's name to the name of the bucket
you created earlier. Using the "Add Statement" button, this statement can be
saved. Now a second statement is created:

```
Effect: Allow
Service: S3
Actions: ListBucket
Resource: arn:aws:s3:::restic-demo
```

Again, substitute `restic-demo` with the actual name of your bucket. Note that,
unlike before, there is no `/*` after the bucket name. This statement allows
Restic to list the objects stored in the `restic-demo` bucket. Again, use "Add
Statement" to save this statement. The policy creator interface should now
look as follows:
![Policy Creator With Two Statements](img/s3/12_policy_permissions_done.png?raw=true)

Continue to the next step and enter a name and description for this policy. For
this tutorial, the policy will be named `restic-demo-policy`. In this step you
can also examine the JSON document created by the policy generator. Click
"Create Policy" to finish the process:
![Policy Review](img/s3/13_policy_review.png?raw=true)

Go back to the browser window or tab where you were previously creating the new
user. Click the button labeled "Refresh" above the list of policies to make
sure the newly created policy is available to you. Afterwards, use the search
function to search for the `restic-demo-policy`. Select this policy using the
checkbox on the left. Then, continue to the next step.
![Attaching Policy To User](img/s3/14_user_attach_policy.png?raw=true)

The next page will present an overview of the user account that is about to be
created. If everything looks good, click "Create user" to complete the process:
![User Creation Review](img/s3/15_user_review.png?raw=true)

After the user has been created, its access credentials will be displayed. They
consist of the "Access key ID" (think username), and the "Secret access key"
(think password). Copy these down to a safe place.
![User Credentials](img/s3/16_user_created.png?raw=true)

You have now completed the configuration in AWS. Feel free to close your web
browser now.


## Initializing the Restic repository

Open a terminal and make sure you have the `restic` binary ready. First, choose
a password to encrypt your backups with. In this tutorial, `apg` is used for
this purpose:

```console
$ apg -a 1 -m 32 -n 1 -M NCL
I9n7G7G0ZpDWA3GOcJbIuwQCGvGUBkU5
```

Note this password somewhere safe along with your AWS credentials. Next, the
configuration of Restic will be placed into environment variables. This will
include sensitive information, such as your AWS secret and repository password.
Therefore, make sure the next commands **do not** end up in your shell's
history file. Adjust the contents of the environment variables to fit your
bucket's name and your user's API credentials.

```console
$ unset HISTFILE
$ export RESTIC_REPOSITORY="s3:https://s3.amazonaws.com/restic-demo"
$ export AWS_ACCESS_KEY_ID="AKIAJAJSLTZCAZ4SRI5Q"
$ export AWS_SECRET_ACCESS_KEY="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"
$ export RESTIC_PASSWORD="I9n7G7G0ZpDWA3GOcJbIuwQCGvGUBkU5"
```

After the environment is set up, Restic may be called to initialize the
repository:


```console
$ ./restic init
created restic backend b5c661a86a at s3:https://s3.amazonaws.com/restic-demo

Please note that knowledge of your password is required to access
the repository. Losing your password means that your data is
irrecoverably lost.
```

Restic is now ready to be used with AWS S3. Try to create a backup:

```console
$ dd if=/dev/urandom bs=1M count=10 of=test.bin
10+0 records in
10+0 records out
10485760 bytes (10 MB, 10 MiB) copied, 0,0891322 s, 118 MB/s

$ ./restic backup test.bin
scan [/home/philip/restic-demo/test.bin]
scanned 0 directories, 1 files in 0:00
[0:04] 100.00%  2.500 MiB/s  10.000 MiB / 10.000 MiB  1 / 1 items ... ETA 0:00 
duration: 0:04, 2.47MiB/s
snapshot 10fdbace saved

$ ./restic snapshots
ID        Date                 Host        Tags        Directory
----------------------------------------------------------------------
10fdbace  2017-03-26 16:41:50  blackbox                /home/philip/restic-demo/test.bin
```

A snapshot was created and stored in the S3 bucket. This snapshot may now be
restored:

```console
$ mkdir restore

$ ./restic restore 10fdbace --target restore
restoring <Snapshot 10fdbace of [/home/philip/restic-demo/test.bin] at 2017-03-26 16:41:50.201418102 +0200 CEST by philip@blackbox> to restore

$ ls restore/
test.bin
```

The snapshot was successfully restored. This concludes the tutorial.

