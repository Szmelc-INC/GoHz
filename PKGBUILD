# Maintainer: SilverX <serainox@gmail.com>
pkgname=gohz-git
_repo=GoHz
pkgver=r0.0000000
pkgrel=1
pkgdesc="Fast audio analysis & manipulation toolkit in Go (gohz-analize / gohz-split)"
arch=('x86_64' 'aarch64')
url="https://github.com/Szmelc-INC/GoHz"
license=('MIT')
depends=('ffmpeg')
optdepends=('aubio: tempo/pitch/key analysis'
            'demucs: AI stem separation for gohz-split')
makedepends=('git' 'go')
provides=('gohz')
conflicts=('gohz')
options=('!debug')
source=("git+$url.git")
sha256sums=('SKIP')

pkgver() {
  cd "$_repo"
  ( set -o pipefail
    git describe --long --tags --abbrev=7 2>/dev/null | sed 's/^v//;s/\([^-]*-g\)/r\1/;s/-/./g' ||
    printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short=7 HEAD)" )
}

prepare() {
  cd "$_repo"
  export GOPATH="$srcdir/gopath" GOFLAGS="-modcacherw"
  go mod download
}

build() {
  cd "$_repo"
  export GOPATH="$srcdir/gopath" CGO_ENABLED=0 GOFLAGS="-trimpath -mod=readonly -modcacherw"
  go build -ldflags "-s -w" -o gohz-analize ./analize
  go build -ldflags "-s -w" -o gohz-split ./split
}

check() {
  cd "$_repo"
  export GOPATH="$srcdir/gopath" CGO_ENABLED=0 GOFLAGS="-mod=readonly -modcacherw"
  HOME="$srcdir" go test ./analize ./split
}

package() {
  cd "$_repo"
  install -Dm755 gohz-analize "$pkgdir/usr/bin/gohz-analize"
  install -Dm755 gohz-split "$pkgdir/usr/bin/gohz-split"
  install -Dm644 README.md "$pkgdir/usr/share/doc/$pkgname/README.md"
  install -Dm644 analize/README.md "$pkgdir/usr/share/doc/$pkgname/analize-README.md"
  install -Dm644 split/README.md "$pkgdir/usr/share/doc/$pkgname/split-README.md"
  install -Dm644 LICENSE "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
}
