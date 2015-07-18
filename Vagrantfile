# -*- mode: ruby -*-
# vi: set ft=ruby :

GO_VERSION = '1.4.2'

def packages_freebsd
  return <<-EOF
    pkg install -y git
    pkg install -y curl
  EOF
end

def packages_openbsd
  return <<-EOF
    . ~/.profile
    pkg_add git curl bash gtar--
    ln -sf /usr/local/bin/gtar /usr/local/bin/tar
  EOF
end

def packages_linux
  return <<-EOF
    apt-get update
    apt-get install -y git curl
  EOF
end

def install_gimme
  return <<-EOF
    rm -rf /opt/gimme
    git clone https://github.com/meatballhat/gimme /opt/gimme
    perl -p -i -e 's,/bin/bash,/usr/bin/env bash,' /opt/gimme/gimme
    ln -sf /opt/gimme/gimme /usr/bin/gimme
  EOF
end

def prepare_user(boxname)
  return <<-EOF
    mkdir -p ~/go/src
    export PATH=/usr/local/bin:$PATH

    gimme #{GO_VERSION} >> ~/.profile
    echo export 'GOPATH=$HOME/go' >> ~/.profile
    echo export 'CDPATH=.:$GOPATH/src/github.com' >> ~/.profile
    echo export 'PATH=$GOPATH/bin:/usr/local/bin:$PATH' >> ~/.profile

    . ~/.profile

    go get golang.org/x/tools/cmd/cover
    go get github.com/tools/godep

    echo
    echo "Run:"
    echo "  vagrant rsync #{boxname}"
    echo "  vagrant ssh #{boxname} -c 'cd project/path; godep go test ./...'"
  EOF
end

def fix_perms
  return <<-EOF
    chown -R vagrant:vagrant /home/vagrant/go
  EOF
end

# All Vagrant configuration is done below. The "2" in Vagrant.configure
# configures the configuration version (we support older styles for
# backwards compatibility). Please don't change it unless you know what
# you're doing.
Vagrant.configure(2) do |config|
  # disable default mount
  config.vm.synced_folder ".", "/vagrant", :disabled => true

  # use rsync to copy content to the folder
  config.vm.synced_folder ".", "/home/vagrant/go/src/github.com/restic/restic", :type => "rsync"

  # fix permissions on synced folder
  config.vm.provision "fix perms", :type => :shell, :inline => fix_perms

  config.vm.define "linux" do |b|
    b.vm.box = "ubuntu/trusty64"
    b.vm.provision "packages linux", :type => :shell, :inline => packages_linux
    b.vm.provision "install gimme",  :type => :shell, :inline => install_gimme
    b.vm.provision "prepare user",   :type => :shell, :privileged => false, :inline => prepare_user("linux")
  end

  config.vm.define "freebsd" do |b|
    b.vm.box = "geoffgarside/freebsd-10.1"
    b.vm.provision "packages freebsd", :type => :shell, :inline => packages_freebsd
    b.vm.provision "install gimme",  :type => :shell, :inline => install_gimme
    b.vm.provision "prepare user",   :type => :shell, :privileged => false, :inline => prepare_user("freebsd")
  end

  config.vm.define "openbsd" do |b|
    b.vm.box = "tmatilai/openbsd-5.6"
    b.vm.provision "packages openbsd", :type => :shell, :inline => packages_openbsd
    b.vm.provision "install gimme",  :type => :shell, :inline => install_gimme
    b.vm.provision "prepare user",   :type => :shell, :privileged => false, :inline => prepare_user("openbsd")
  end
end
