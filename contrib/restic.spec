Name: restic	
Version: 0.9.2git.20180812	
Release: 4%{?dist}
Summary: restic is a backup program that is fast, efficient and secure.

%global debug_package %{nil}

Group: Applications/Archiving
License: BSD 2-Clause
URL: https://restic.net/	
Source0: %{name}-%{version}.tar.gz

BuildRequires:	golang 
Requires: /bin/bash	

%description
restic is a program that does backups right. The design goals are:

Easy: Doing backups should be a frictionless process, otherwise you are tempted to skip it. Restic should be easy to configure and use, so that in the unlikely event of a data loss you can just restore it. Likewise, restoring data should not be complicated.

Fast: Backing up your data with restic should only be limited by your network or hard disk bandwidth so that you can backup your files every day. Nobody does backups if it takes too much time. Restoring backups should only transfer data that is needed for the files that are to be restored, so that this process is also fast.

Verifiable: Much more important than backup is restore, so restic enables you to easily verify that all data can be restored.

Secure: Restic uses cryptography to guarantee confidentiality and integrity of your data. The location where the backup data is stored is assumed to be an untrusted environment (e.g. a shared space where others like system administrators are able to access your backups). Restic is built to secure your data against such attackers, by encrypting it with AES-256 in counter mode and authenticating it using Poly1305-AES.

Efficient: With the growth of data, additional snapshots should only take the storage of the actual increment. Even more, duplicate data should be de-duplicated before it is actually written to the storage backend to save precious backup space.

Versatile storage: Users can provide many different places to store the backups. Local, SFTP, Restics REST-Server, Amazon S3, Minio, Openstack Swift, Backblaze B2, Microsoft Azure Blob Storage, Google Cloud Storage and more by the usage of rclone.

Free: restic is free software and licensed under the BSD 2-Clause License and actively developed on GitHub.

%prep
%setup -q


%build
make %{?_smp_mflags}


%install
mkdir -p %{buildroot}%{_bindir}
mkdir -p %{buildroot}%{_mandir}/man1
mkdir -p %{buildroot}%{_datarootdir}/zsh/site-functions
mkdir -p %{buildroot}%{_datarootdir}/bash-completion/completions
install -p -m 644 doc/man/* %{buildroot}%{_mandir}/man1/
install -p -m 644 doc/zsh-completion.zsh %{buildroot}%{_datarootdir}/zsh/site-functions/_restic
install -p -m 644 doc/bash-completion.sh %{buildroot}%{_datarootdir}/bash-completion/completions/restic
install -p -m 755 %{name} %{buildroot}%{_bindir}

%files
%doc LICENSE
%doc README.rst
%{_bindir}/%{name}
%dir %{_datadir}/zsh/site-functions
%{_datadir}/zsh/site-functions/_restic
%dir %{_datadir}/bash-completion/
%dir %{_datadir}/bash-completion/completions
%{_datadir}/bash-completion/completions/restic
%{_mandir}/man1/restic*.*


%changelog
* Sun Aug 12 2018 Luc De Louw <luc@delouw.ch> - 0.9.2git.20180812-4
- %license does not work with RHEL6, using %doc instead
* Sun Aug 12 2018 Luc De Louw <luc@delouw.ch> - 0.9.2git.20180812-3
- Better description
* Sun Aug 12 2018 Luc De Louw <luc@delouw.ch> - 0.9.2git.20180812-2
- Initial RPM build
* Sun Aug 12 2018 Luc De Louw <luc@delouw.ch> - 0.9.2git.20180812-1
- Initial RPM build

