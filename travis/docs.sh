#!/usr/bin/env bash

set -e
TAGNAME=$1
GH_REF=github.com/aerokube/selenoid.git
git config user.name "${TRAVIS_REPO_SLUG}"
git config user.email "aerokube@aerokube.github.com"
git remote add upstream "https://${GITHUB_TOKEN}@${GH_REF}"
git fetch upstream

git branch -r

echo "Deleting old output"
rm -rf ${TRAVIS_BUILD_DIR}/docs/output
mkdir ${TRAVIS_BUILD_DIR}/docs/output
git worktree prune
rm -rf ${TRAVIS_BUILD_DIR}/.git/worktrees/docs/output/

echo "Checking out gh-pages branch into docs/output"
git worktree add -B gh-pages ${TRAVIS_BUILD_DIR}/docs/output upstream/gh-pages

echo "Removing existing files"
mkdir -p ${TRAVIS_BUILD_DIR}/docs/output/${TAGNAME}
rm -rf ${TRAVIS_BUILD_DIR}/docs/output/${TAGNAME}/*

echo "Copying images"
cp -R ${TRAVIS_BUILD_DIR}/docs/img ${TRAVIS_BUILD_DIR}/docs/output/${TAGNAME}/img
echo "Generating docs"
docker run -v ${TRAVIS_BUILD_DIR}/docs/:/documents/ --name asciidoc-to-html asciidoctor/docker-asciidoctor asciidoctor -a revnumber=${TAGNAME} -D /documents/output/${TAGNAME} index.adoc


echo "Updating gh-pages branch"
cd ${TRAVIS_BUILD_DIR}/docs/output && git add --all && git commit -m "Publishing to gh-pages"


git push upstream HEAD:gh-pages
