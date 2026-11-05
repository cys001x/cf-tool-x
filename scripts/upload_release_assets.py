#!/usr/bin/env python3
"""Upload release assets to a GitHub release.

The asset names must match what `cf upgrade` builds, otherwise the upgrade
command cannot find the download URL:

  cf_<tag>_<goos>_<32|64>.zip   e.g. cf_v1.0.6_darwin_64.zip

Build them with:

  for os in darwin linux windows; do
    for arch in amd64 386; do
      bits=64; [ $arch = 386 ] && bits=32
      out=cf; [ $os = windows ] && out=cf.exe
      GOOS=$os GOARCH=$arch go build -o "$out" .
      zip -j "cf_${TAG}_${os}_${bits}.zip" "$out" && rm -f "$out"
    done
  done

Usage:
  export GITHUB_TOKEN=<github personal access token>
  ./upload_release_assets.py --tag v1.0.6 dist/*.zip

  # upload to another repository
  ./upload_release_assets.py --repo owner/name --tag v1.0.0 dist/*.zip

If the release for the tag does not exist yet, the script creates it with
auto-generated release notes. Existing assets are skipped unless --clobber is
given, in which case they are deleted and uploaded again.
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

API = "https://api.github.com"
UPLOADS = "https://uploads.github.com"


def parse_args():
    parser = argparse.ArgumentParser(description="Upload assets to a GitHub release")
    parser.add_argument("--repo", default="cys001x/cf-tool-x",
                        help="owner/name (default: cys001x/cf-tool-x)")
    parser.add_argument("--tag", required=True, help="release tag, e.g. v1.0.6")
    parser.add_argument("--clobber", action="store_true",
                        help="replace assets that already exist on the release")
    parser.add_argument("files", nargs="+", help="asset files to upload")
    return parser.parse_args()


def request(method, url, token, data=None, headers=None):
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", "Bearer " + token)
    req.add_header("Accept", "application/vnd.github+json")
    req.add_header("X-GitHub-Api-Version", "2022-11-28")
    for key, value in (headers or {}).items():
        req.add_header(key, value)
    with urllib.request.urlopen(req) as resp:
        body = resp.read()
        return resp.status, json.loads(body) if body else None


def get_release(repo, tag, token):
    try:
        _, release = request(
            "GET", "%s/repos/%s/releases/tags/%s" % (API, repo, urllib.parse.quote(tag)), token)
        return release
    except urllib.error.HTTPError as err:
        if err.code != 404:
            raise
        return None


def create_release(repo, tag, token):
    payload = json.dumps({
        "tag_name": tag,
        "name": tag,
        "generate_release_notes": True,
    }).encode()
    _, release = request(
        "POST",
        "%s/repos/%s/releases" % (API, repo),
        token,
        data=payload,
        headers={"Content-Type": "application/json"},
    )
    print("created release %s" % tag)
    return release


def find_asset(release, name):
    for asset in release.get("assets", []):
        if asset["name"] == name:
            return asset
    return None


def delete_asset(repo, asset, token):
    request("DELETE", "%s/repos/%s/releases/assets/%d" % (API, repo, asset["id"]), token)


def upload_asset(repo, release_id, path, name, token):
    data = open(path, "rb").read()
    url = "%s/repos/%s/releases/%d/assets?name=%s" % (
        UPLOADS, repo, release_id, urllib.parse.quote(name))
    status, _ = request(
        "POST", url, token, data=data,
        headers={"Content-Type": "application/zip"})
    return status


def main():
    args = parse_args()
    token = os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN")
    if not token:
        sys.exit("Set GITHUB_TOKEN (or GH_TOKEN) to a GitHub personal access token")

    release = get_release(args.repo, args.tag, token)
    if release is None:
        release = create_release(args.repo, args.tag, token)

    failed = []
    for path in args.files:
        name = os.path.basename(path)
        try:
            existing = find_asset(release, name)
            if existing is not None:
                if not args.clobber:
                    print("%s: already exists (use --clobber to replace)" % name)
                    continue
                delete_asset(args.repo, existing, token)
            status = upload_asset(args.repo, release["id"], path, name, token)
            print("%s: %s" % (name, status))
        except urllib.error.HTTPError as err:
            print("%s: failed %s %s" % (name, err.code, err.read()[:200]))
            failed.append(name)
        except OSError as err:
            print("%s: failed %s" % (name, err))
            failed.append(name)

    if failed:
        sys.exit("Failed to upload: %s" % ", ".join(failed))


if __name__ == "__main__":
    main()
