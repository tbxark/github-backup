# Github backup

Backup all github repos, (This is a [zx](https://github.com/google/zx) based script)


### Quick usage

```shell
npx zx https://cdn.jsdelivr.net/gh/tbxark/github-backup/legacy/backup.mjs
npx zx https://cdn.jsdelivr.net/gh/tbxark/github-backup/legacy/mirror.mjs
```


### Install


```shell
npm i -g zx

wget https://cdn.jsdelivr.net/gh/tbxark/github-backup/legacy/backup.mjs
wget https://cdn.jsdelivr.net/gh/tbxark/github-backup/legacy/mirror.mjs

chmod +x backup.mjs mirror.mjs

./backup.mjs --config=$(PATH_TO_CONFIG) --target=$(PATH_TO_STORE_DIR) 
./mirror.mjs --config=$(PATH_TO_CONFIG) --target=$(PATH_TO_STORE_DIR) --token=${GITEE_TOKEN}
```


### Option

- #### `target` 
  The target directory to store the backup. Default is `./`.
  
- #### `config`
  The config file, the default is `./github_backup_config.json`, create a config file if not exists
  
- #### `clone`
  > Opitions: `all`, `none`, `question`<br>
  > Usage in `backup.mjs` only

  Whether to clone the repos, the default is `all`

- #### `untracked`
  > Opitions: `question`, `delete`, `keep`<br>
  > Usage in `backup.mjs` only

  Whether to delete untracked repos, the default is `question`

- #### `branch`
  > Opitions: `all`, `current`<br>
  > Usage in `backup.mjs` only

  When `branch` is `all`, clone all branches. When `branch` is `current`, clone only the current branch.
  
- #### `token`: 
  > Usage in `mirror.mjs` only
  
  The token to access Gitee.

- #### `private`:
  > Opitions: `always`, `auto`<br>
  > Usage in `mirror.mjs` only

  create private repos or not, the default is `auto`
  
### Configuration

Configuration file will auto generate if not exists. You can edit the config file to change value.

```js
{
  "username": "tbxark",
  "token": "YOUR_TOKEN", // https://github.com/settings/tokens
  "repos": {
    "TKRubberIndicator": {
      "name": "TKRubberIndicator",
      "ignore": false, // If true, do not clone to the local
      "keep": true, // If true, keep a local backup when the remote repo is deleted
      "status": {
        "private": false,
        "fork": false,
        "archived": false
      },
      "date": {
        "created_at": "2015-10-28T02:14:22Z",
        "updated_at": "2022-02-07T08:09:48Z"
      },
      "ssh_url": "git@github.com:TBXark/TKRubberIndicator.git",
    }
   }
 }
```

---



## 中文


### 备份

使用备份脚本需要`config`配置文件路径，当配置文件不存在的时候会自动创建。`target`是存储repo的目录，`clone`可以选择：`all`下载所有repo，`none`忽略未下载的repo，或者其他值根据repo手动选择。
当github的token过期的时候可以手动修改配置文件中的token值。或者某个repo的配置有问题也可以手动的修改配置文件。

```shell
./backup.mjs --config=$(PATH_TO_CONFIG) --target=$(PATH_TO_STORE_DIR) --clone=${all|none|question}
```

### 镜像

暂时只写了一个将repos镜像到gitee的脚本, 这个得clone本仓库后使用，需要gitee的[token](https://gitee.com/profile/personal_access_tokens)，会自动创建不存在的repo。
可以根据原有github仓库类型创建私有还是公有仓库（公有仓库得手机号码认证），也可以全部设置为私有。
```shell
./mirror.mjs --config=$(PATH_TO_CONFIG) --target=$(PATH_TO_STORE_DIR) --token=${GITEE_TOKEN} --private=${always|auto}
```

### 碎碎念

最近看得到v2ex上有人github账户被封，俄罗斯也被米国盟友各种制裁。感觉把所有代码放在Github上不备份其实不太安全。
现有其他git平台不能实时同步github的仓库。得在每个github的repo添加action去同步，过于麻烦。
所以还是先备份到本地，然后再同步到另外的git平台。而且本地多一个备份也比较安全。
