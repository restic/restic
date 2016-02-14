# Maintainer: Florian Daniel <fd@noxa.de>
# Contributor: Eldar Tsraev <elts@culab.org>
# Contributor: Andreas Guth <andreas.guth@rwth-aachen.de>
# Contributor: Alexander Neumann <alexander@bumpern.de>
options=(!strip)
pkgname=restic-git
pkgver=v0.1.0.r172.g1f1b8e1
pkgrel=1
pkgdesc="restic is a program that does backups right."
arch=('i686' 'x86_64')
url="https://github.com/restic/restic"
license=('BSD')
depends=('glibc')
makedepends=('git' 'go>=1.3')
provides=('restic')
conflicts=('restic')
source=("${pkgname}::git+https://github.com/restic/restic")
md5sums=('SKIP')

importpath='github.com/restic/restic'

pkgver() {
  cd "$pkgname"
  git describe --long | sed 's/\([^-]*-g\)/r\1/;s/-/./g'
}

build() {
  cd "$pkgname"
  go run build.go
}

package() {
  install -Dm755 "$pkgname/restic"    "$pkgdir/usr/bin/restic"
  install -Dm644 "$pkgname/LICENSE"   "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
  install -Dm644 "$pkgname/README.md" "$pkgdir/usr/share/doc/$pkgname/README"
}

# vim:set ts=2 sw=2 et:
