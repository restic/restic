# Maintainer: Eldar Tsraev <elts@culab.org>
pkgname=restic-git
pkgver=
pkgrel=1
pkgdesc="Restic is a program that does backups right."
arch=('i686' 'x86_64')
url="https://github.com/restic/restic"
license=('BSD')
depends=('glibc')
makedepends=('git' 'go')
provides=('restic')
conflicts=('restic')
source=('restic-git::git+https://github.com/restic/restic')
md5sums=('SKIP') #generate with 'makepkg -g'

pkgver() {
  cd "$srcdir/$pkgname"
  printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)";
}

prepare() {
  # git repo fetched by makepkg and it is possible to calculate a fancy version number.
  # On the other hand we don't need to re-download source code with go-get...
  export GOPATH="$srcdir"
  export gorepo="github.com/restic/restic"
  mkdir -p $GOPATH/src/github.com/restic
  rm -rf $GOPATH/src/$gorepo
  mv $srcdir/$pkgname $GOPATH/src/$gorepo
}

build() {
  go get -v $gorepo/cmd/restic
}

package() {
  # Copying file(s)
  install -Dm755 $GOPATH/bin/restic $pkgdir/usr/bin/restic
  # Copying LICENCE file
  install -Dm644 $GOPATH/src/$gorepo/LICENSE $pkgdir/usr/share/licenses/$pkgname/LICENSE
}

# vim:set ts=2 sw=2 et:
