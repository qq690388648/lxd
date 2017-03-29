#!/bin/sh

safe_pot_hash() {
  sed -e "/Project-Id-Version/,/Content-Transfer-Encoding/d" -e "/^#/d" "po/lxd.pot" | md5sum | cut -f1 -d" "
}

test_static_analysis() {
  (
    set -e

    cd ../
    # Python3 static analysis
    pep8 test/deps/import-busybox
    pyflakes3 test/deps/import-busybox

    # Shell static analysis
    shellcheck test/main.sh test/suites/* test/backends/*

    # Go static analysis
    ## Functions starting by empty line
    OUT=$(grep -r "^$" -B1 . | grep "func " | grep -v "}$" || true)
    if [ -n "${OUT}" ]; then
      echo "ERROR: Functions must not start with an empty line: ${OUT}"
      false
    fi

    ## go vet, if it exists
    if go help vet >/dev/null 2>&1; then
      go vet ./...
    fi

    ## vet
    if which vet >/dev/null 2>&1; then
      vet --all .
    fi

    ## golint
    if which golint >/dev/null 2>&1; then
      golint -set_exit_status shared/api/
    fi

    ## deadcode
    if which deadcode >/dev/null 2>&1; then
      OUT=$(deadcode ./ ./fuidshift ./lxc ./lxd ./lxd/types ./shared ./shared/api ./shared/i18n ./shared/ioprogress ./shared/logging ./shared/osarch ./shared/simplestreams ./shared/termios ./shared/version ./test/lxd-benchmark 2>&1 | grep -v lxd/migrate.pb.go: | grep -v /C: | grep -vi _cgo | grep -vi _cfunc || true)
      if [ -n "${OUT}" ]; then
        echo "${OUT}" >&2
        false
      fi
    fi

    if which godeps >/dev/null 2>&1; then
      OUT=$(godeps . ./shared | cut -f1)
      if [ "${OUT}" != "$(printf "github.com/gorilla/websocket\ngopkg.in/yaml.v2\n")" ]; then
        echo "ERROR: you added a new dependency to the client or shared; please make sure this is what you want"
        echo "${OUT}"
      fi
    fi

    # Skip the tests which require git
    if ! git status; then
      return
    fi

    # go fmt
    git add -u :/
    gofmt -w -s ./
    git diff --exit-code

    # make sure the .pot is updated
    cp --preserve "po/lxd.pot" "po/lxd.pot.bak"
    hash1=$(safe_pot_hash)
    make i18n -s
    hash2=$(safe_pot_hash)
    mv "po/lxd.pot.bak" "po/lxd.pot"
    if [ "${hash1}" != "${hash2}" ]; then
      echo "==> Please update the .pot file in your commit (make i18n)" && false
    fi
  )
}
