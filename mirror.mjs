#!/usr/bin/env zx

// import "zx/globals";

// /////////////Func/////////////////

function parseArgs() {
  const res = argv;
  delete res['_'];

  if (res.target) {
    res.target = path.resolve(res.target);
  } else {
    res.target = path.resolve('.');
  }
  if (res.config) {
    res.config = path.resolve(res.config);
  } else {
    res.config = path.resolve('./.github_backup_config.json');
  }
  return res;
}

async function createGiteeRepoIfNotExist(name, user, token, isPrivate) {
  let response = await fetch(
      `https://gitee.com/api/v5/repos/${user}/${name}?access_token=${token}`,
      {
        method: 'GET',
      },
  );
  if (response.ok) {
    return await response.json();
  }
  response = await fetch(
      `https://gitee.com/api/v5/user/repos?name=${name}&access_token=${token}&private=${isPrivate}`,
      {
        method: 'POST',
      },
  );
  return await response.json();
}

// /////////////Main/////////////////

const args = parseArgs();
console.log(args);

if (!args.token) {
  throw new Error('Missing token');
}

const config = await fs.readFile(args.config);

for (const name of Object.keys(config.repos)) {
  cd(args.target);
  const repo = config.repos[name];
  if (repo.ignore) {
    continue;
  }
  const repoPath = path.resolve(`./${repo.name}`);
  const mRepo = await createGiteeRepoIfNotExist(
      name,
      config.username,
      token,
    args.private === 'always' ? true : repo.status.private,
  );
  if (fs.existsSync(repoPath)) {
    cd(repoPath);
    if (mRepo.ssh_url) {
      await $`git push --mirror ${mRepo.ssh_url}`;
    }
  }
}
