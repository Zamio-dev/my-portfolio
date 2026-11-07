# my-portfolio

Go based web app deployed end to end with Docker, Kubernetes and Helm on AWS EKS. GitHub Actions builds, tests and lints the code, pushes the image to Docker Hub and updates the Helm chart tag. ArgoCD picks up the new tag and rolls it out.

![go-web-app home page served from the EKS cluster at go-web-app.local](docs/screenshot-home.png)

## What this project all about

A case study. I followed Abhishek Veeramalla's [go-web-app-devops](https://github.com/iam-veeramalla/go-web-app-devops) course and devopsified a small Go app: container, Kubernetes manifests, Helm chart, CI/CD and GitOps. The app is not the point. The flow around it is.

- The app is a Go `net/http` server. It serves static HTML pages on port 8080. No database, no Node, no framework.
- The repo is `my-portfolio`, the app is still `go-web-app`. The image, the Helm chart and the Kubernetes objects all use `go-web-app`.
- The Apache-2.0 license from the course repo is kept.

## How it works

Deploy flow:

```
git push to main
   |
   v
GitHub Actions
   |-- build + test
   |-- lint (runs next to build, not after it)
   |-- push image to Docker Hub, tag = run ID
   '-- write the new tag into helm/go-web-app-chart/values.yaml and commit it
   |
   v
ArgoCD (running inside EKS) sees the new commit and syncs
   |
   v
new pod with the new image
```

Request flow:

```
browser -> go-web-app.local -> /etc/hosts -> AWS load balancer -> ingress-nginx -> go-web-app service -> pod
```

## Repo layout

```
.
├── main.go                     the app
├── main_test.go                tests
├── static/                     HTML, CSS and images the app serves
├── Dockerfile                  builds the image
├── k8s/manifest/               deployment, service, ingress as plain YAML
├── helm/go-web-app-chart/      the same objects as a Helm chart
├── .github/workflows/ci.yaml   CI/CD pipeline
├── docs/                       README screenshot
└── go.mod
```

## How to read this README

The steps follow the order you run things in. Each step has:

- the commands, with every option explained
- the other options and why I picked this one
- the issues I hit in that step, with the cause, the fix and how to avoid it

I use three labels:

- **What I used**: what is in this repo.
- **Leanest that works**: the smallest setup that still does the job.
- **Recommended**: what I would do for anything beyond practice.

At a glance:

| Part | What I used | Leanest that works | Recommended |
| --- | --- | --- | --- |
| Cluster | `eksctl`, 2 nodes | same | Terraform |
| Image tag | CI run ID | same | git commit SHA |
| CI writing the Helm tag back | personal access token (`TOKEN`) | built in `GITHUB_TOKEN` | same as leanest |
| Deploy gate | `push` waits for `build` | same | `push` waits for `build` and `code-quality` |
| Ingress controller | ingress-nginx | same | AWS Load Balancer Controller or a Gateway API controller |
| Open the app | hosts file entry | `curl -H "Host: ..."` | Route 53 record + TLS |
| ArgoCD UI | LoadBalancer service | `kubectl port-forward` | port-forward, or behind the ingress with TLS and SSO |
| Deploying | ArgoCD | CI runs `helm upgrade` | ArgoCD or Flux |

**Cost warning.** EKS bills per hour for the control plane, the nodes and every load balancer. This repo creates two load balancers. Do the [teardown](#11-teardown) when you stop for the day.

---

## 0. Prerequisites

Accounts: GitHub, Docker Hub, AWS.

| Tool | Used for | Check |
| --- | --- | --- |
| Go | run and test the app | `go version` |
| Docker | build and run the image | `docker --version` |
| git and `gh` (GitHub CLI) | repo, secrets, workflow logs | `gh --version` |
| `aws` (AWS CLI) | log in to AWS, connect kubectl to EKS | `aws --version` |
| `eksctl` | create and delete the cluster | `eksctl version` |
| `kubectl` | talk to the cluster | `kubectl version --client` |
| `helm` | install ingress-nginx, render the chart | `helm version` |

On macOS with Homebrew: `brew install go gh awscli eksctl kubectl helm` and `brew install --cask docker`.

Log in once:

```bash
gh auth login
docker login
aws configure
aws sts get-caller-identity
```

- `gh auth login` logs the GitHub CLI in through the browser.
- `docker login` asks for your Docker Hub username and an access token.
- `aws configure` asks for an access key, a secret key, a default region and an output format. This repo uses `ap-southeast-2`.
- `aws sts get-caller-identity` prints which AWS account and user you are logged in as. Run it so you don't build in the wrong account.

**Leanest that works:** an IAM user with keys, deleted after you are done. **Recommended:** IAM Identity Center (`aws configure sso`) so there are no long lived keys on your laptop. Never use the root account.

---

## 1. Run it locally first

```bash
go run main.go
go test ./...
go build -o go-web-app
```

- `go run main.go` compiles and starts the server in one go. It listens on port 8080. Open `http://localhost:8080/home`.
- `go test ./...` runs the tests in the current folder and every folder below it. `./...` means "all packages here and under here".
- `go build -o go-web-app` compiles to a binary. `-o` sets the output file name.

If it doesn't work here it won't work in a container or in CI. Fix it here first.

Remember one thing from this step: the handlers read HTML files from the `static/` folder on disk at runtime, and the tests do too. This matters in step 4.

---

## 2. Git and the GitHub repo

Create a `.gitignore` before the first commit:

```
.DS_Store
/main
/go-web-app
kubectl
```

- `.DS_Store` is macOS folder metadata.
- `/main` and `/go-web-app` are the compiled binaries. The leading `/` means "only in the repo root".
- `kubectl` is there because I once dropped the kubectl binary into the repo (see issues below).

Then:

```bash
git init -b main
git add .
git commit -m "Initial commit"
gh repo create my-portfolio --public --source=. --remote=origin --push
```

- `git init -b main` creates the repo and names the first branch `main`.
- `git add .` stages everything in the folder that is not ignored.
- `git commit -m "..."` saves the staged files with a message.
- `gh repo create my-portfolio` creates the repo on GitHub.
  - `--public` makes it public.
  - `--source=.` uses the current folder as the source.
  - `--remote=origin` names the remote `origin`.
  - `--push` pushes the commits right after creating it.

**Other options.** Create the repo in the GitHub website, then `git remote add origin <url>` and `git push -u origin main`. Same result, more clicks. I use `gh` because it is one command and I need it later for secrets and logs anyway. I also removed the course repo's `.git` folder (`rm -rf .git`) and started a fresh history. A fork would keep the course history and credit, which is the other honest way to do it.

**Issues I hit**

> **Branch was `master`, not `main`**
> - Cause: after `git init` Git used its old default name. The workflow only triggers on `main`.
> - Fix: `git branch -m main` renames the current branch.
> - Avoid: `git init -b main`, or once: `git config --global init.defaultBranch main`.

> **`gh repo create` ended with "Unable to add remote origin"**
> - Cause: the local repo already had a remote called `origin`. The GitHub repo was still created.
> - Fix: run `git remote -v` to see what `origin` points to. Then either `git remote set-url origin https://github.com/<you>/my-portfolio.git` or keep the old one under another name with `git remote rename origin upstream` and add the new one.
> - Avoid: on a fresh `git init` there is no `origin`, so the one liner above works. Run `git remote -v` first if you are unsure.

> **`git push` asked for a password and failed with "Password authentication is not supported"**
> - Cause: GitHub doesn't accept your account password over HTTPS. It wants a token.
> - Fix: `gh auth setup-git` makes git use the login that `gh` already has. Then `git push -u origin main`. `-u` links your local `main` to `origin/main`, so a plain `git push` works afterwards.
> - Avoid: run `gh auth login` and `gh auth setup-git` once before the first push.

> **A 60 MB `kubectl` binary, `.DS_Store` files and the compiled `main` binary ended up in the repo**
> - Cause: `git add .` adds everything that is not ignored. I had put the kubectl binary in `k8s/manifest/` while following along.
> - Fix: `git rm --cached <file>` stops tracking a file but keeps it on disk (`-r` for folders). Add the names to `.gitignore`. The old commit still holds the files, so on a fresh solo repo I amend the commit and force push: `git commit --amend` then `git push --force-with-lease origin main`. `--force-with-lease` overwrites the remote only if nobody pushed there since you last looked.
> - Avoid: run `git status` before the first commit and read the list.

Small one: wrong commit message on the last commit. `git commit --amend -m "new message"` rewrites it. If it is already pushed, add `git push --force-with-lease origin main`.

---

## 3. The Docker image

The `Dockerfile` is multi stage. Stage one uses the Go image to compile the app. Stage two is a small distroless image that only gets the compiled binary and the `static/` folder. The Go compiler never reaches the final image.

```bash
docker build -t <dockerhub-user>/go-web-app:test .
docker run --rm -p 8080:8080 <dockerhub-user>/go-web-app:test
```

- `docker build` builds an image from the `Dockerfile`.
  - `-t name:tag` names and tags the image.
  - `.` is the build context, the folder Docker is allowed to copy from.
- `docker run` starts a container from the image.
  - `--rm` deletes the container when it stops.
  - `-p 8080:8080` maps port 8080 on your machine to port 8080 in the container (`host:container`).

The app listens on `0.0.0.0:8080`, not `127.0.0.1`. Inside a container `127.0.0.1` is only the container itself, so nothing outside could reach it.

**Other options**

- Buildpacks (`pack build`) and `ko` build an image without a Dockerfile. `ko` is very lean for Go. I used a Dockerfile because it works for any language and every team can read one.
- Final image base: `scratch` is the smallest but has nothing in it. `alpine` has a shell and a package manager. `distroless` sits in the middle: no shell, no package manager, but CA certificates are there. That is why I picked it.

**Recommended:** pin the base image versions, run as a non-root user (distroless has a `:nonroot` tag), and scan the image (`docker scout quickview`).

---

## 4. CI/CD with GitHub Actions

The pipeline is in `.github/workflows/ci.yaml`. The folder name has to be exactly `.github/workflows`.

| Job | What it does | Waits for |
| --- | --- | --- |
| `build` | `go build` and `go test ./...` | nothing |
| `code-quality` | runs `golangci-lint` | nothing |
| `push` | logs in to Docker Hub, builds the image, pushes it | `build` |
| `update-newtag-in-helm-chart` | writes the new tag into `values.yaml`, commits and pushes | `push` |

Trigger:

```yaml
on:
  push:
    branches: [main]
    paths-ignore:
      - 'helm/**'
      - 'k8s/**'
      - 'README.md'
```

`paths-ignore` is not optional. The last job pushes a commit to `main` (the tag change in `helm/`). Without it, that commit would start the pipeline again, which pushes again, forever. It also means README edits don't build anything.

The last job changes the tag with:

```bash
sed -i 's/tag: .*/tag: "<run-id>"/' helm/go-web-app-chart/values.yaml
```

- `sed` edits text. `-i` edits the file in place.
- `s/old/new/` replaces text. `tag: .*` matches `tag:` and the rest of the line.
- On macOS `sed -i` needs an empty string after it (`sed -i ''`). The GitHub runner is Linux, so it doesn't.

### Secrets

```bash
gh secret set DOCKERHUB_USERNAME
gh secret set DOCKERHUB_TOKEN
gh secret set TOKEN
```

`gh secret set NAME` asks you for the value and stores it encrypted in the repo. You can't read it back later, only replace it.

- `DOCKERHUB_USERNAME`: your Docker Hub username.
- `DOCKERHUB_TOKEN`: an access token from Docker Hub, Account settings, Personal access tokens. Give it read and write.
- `TOKEN`: a GitHub personal access token with write access to this repo's contents. The last job uses it to push the tag commit.

Without these the run fails at the Docker login step. The build, test and lint jobs don't need them.

### Choices

**Image tag.** I tag with the run ID: unique, free, nothing to set up. `latest` is the one to avoid: you can't tell what is running, and Kubernetes won't pull a new image when the tag text didn't change. **Recommended:** the git commit SHA (`${{ github.sha }}`), because it points to the exact code. The run ID tells you nothing about the code.

**Pushing the tag back.** The course uses a personal access token and so did I. The leaner way is the token GitHub already gives every run: add `permissions: contents: write` to that job, drop the `token:` line from the checkout step and use `github-actions[bot]` as the git user. No secret to create or rotate.

**Deploy gate.** As written, `push` only waits for `build`. If lint fails, the image still gets built and the tag still changes. **Recommended:** `needs: [build, code-quality]`.

**Versions.** The workflow reads the Go version from `go.mod` (`go-version-file: go.mod`), so it can't drift from the module. **Recommended:** pin `golangci-lint` to a version instead of `latest` so a new rule can't break a green build, and pin actions to a full version.

**Other CI tools.** Jenkins, GitLab CI and CircleCI do the same job. GitHub Actions is already inside the repo, so there is nothing to host.

### Debugging a run

```bash
gh run list --limit 1
gh run view --log-failed
```

- `gh run list --limit 1` shows the latest run.
- `gh run view --log-failed` prints only the logs of the failed steps. Read this before guessing.

### Issues I hit

> **Actions tab showed the "get started" page, no workflow**
> - Cause: the folder was `.github/workflow`. GitHub only reads `.github/workflows`, with the `s`.
> - Fix: `git mv .github/workflow .github/workflows`, commit, push.
> - Avoid: run `git ls-files .github`. The path has to be `.github/workflows/<name>.yaml`.

> **Push rejected: "refusing to allow an OAuth App to create or update workflow ... without `workflow` scope"**
> - Cause: the login git borrows from `gh` had no `workflow` scope. GitHub protects workflow files with an extra permission.
> - Fix: `gh auth refresh -h github.com -s workflow`. `refresh` updates the existing login, `-h` is the host, `-s` adds a scope. Then push again.
> - Avoid: log in with it from the start: `gh auth login -s workflow`.

> **`golangci-lint exit with code 3` and `build` exit code 1**
> - Cause: the course workflow pins old versions (Go 1.22, golangci-lint v1.56.2). They didn't fit the Go version my module uses. Exit code 3 means the linter could not run at all, which is different from finding style problems.
> - Fix: `go-version-file: go.mod` in `setup-go`, a Go setup step in the lint job too, and a newer `golangci-lint-action`.
> - Avoid: never write the Go version in two places.

> **`go test` failed: `got 404 want 200` and `got text/plain; want text/html`**
> - Cause: I had put `static/` in `.gitignore` to keep it local only. The CI checkout had no `static/` folder, so `http.ServeFile` returned a 404 page. It passed on my Mac because the folder was there. The Dockerfile copies `static/` too, so the image build would have broken the same way.
> - Fix: remove `static/` from `.gitignore` and commit the folder.
> - Avoid: if the app reads a file at runtime, the file has to be in git. Quick check: `git clone` your repo into a temp folder and run `go test ./...` there. If it fails there, CI will fail.

> **Warnings about Node.js 20 and `ubuntu-latest`**
> - Cause: notices from GitHub about the runtime of the actions and an upcoming runner image change.
> - Fix: none needed. They are not the reason the run is red. Open the red step and read that.

---

## 5. The EKS cluster

```bash
eksctl create cluster --name demo-cluster --region ap-southeast-2 --nodes 2 --node-type t3.medium
aws eks update-kubeconfig --name demo-cluster --region ap-southeast-2
kubectl get nodes
```

- `eksctl create cluster` builds the whole thing: the EKS control plane, a VPC and a node group. It takes around 15 to 20 minutes.
  - `--name` is the cluster name.
  - `--region` is the AWS region.
  - `--nodes 2` is the number of worker nodes.
  - `--node-type` is the EC2 size of each node. Two small nodes are enough for this repo. Pick your own size.
- `aws eks update-kubeconfig` writes the cluster's address and login into `~/.kube/config`, the file `kubectl` reads.
- `kubectl get nodes` should list the nodes as `Ready`.

**Other options**

- AWS console: clicks, nothing repeatable.
- Terraform: **recommended**. The cluster is code in git, you can review a change and delete it cleanly.
- `kind` or `minikube`: free and local, but there is no cloud to create a load balancer, so step 7 and 8 work differently.

I used `eksctl` because it is the shortest path to a real EKS cluster.

### Issues I hit

> **`kubectl` said "The connection to the server localhost:8080 was refused"**
> - Cause: `kubectl` had no cluster configured, so it falls back to `localhost:8080`. This has nothing to do with the app's port 8080.
> - Fix: `aws eks update-kubeconfig ...` as above.
> - Avoid: run `kubectl config current-context` before anything else. It should print your cluster, not an error.

---

## 6. Deploy by hand with the manifests

```bash
kubectl apply -f k8s/manifest/
kubectl get all
kubectl get ing
```

- `kubectl apply -f <path>` creates or updates the objects in the file or folder. `-f` is file or directory.
- `kubectl get all` lists pods, services, deployments and replica sets in the current namespace.
- `kubectl get ing` lists Ingresses.

Three objects:

- `deployment.yaml` runs the pod from the image.
- `service.yaml` gives the pod one stable address inside the cluster (type `ClusterIP`, port 80).
- `ingress.yaml` is the routing rule: requests for `go-web-app.local` go to that service.

If a pod is not `Running`: `kubectl describe pod <name>` shows events, `kubectl logs <name>` shows the app output. Pods outside `default` need `-n <namespace>`.

This step is for learning what the raw objects look like before Helm and ArgoCD hide them. **Leanest that works:** skip it and go to ArgoCD. If you do it, clean up before ArgoCD takes over the same names: `kubectl delete -f k8s/manifest/`. Two tools fighting over one object is a mess.

Right now `kubectl get ing` shows an empty `ADDRESS`. That is expected. Next step.

---

## 7. The ingress controller

An Ingress is only a rule. Something has to run in the cluster, listen on a public address and apply the rule. That something is the ingress controller. A fresh EKS cluster has none, so the Ingress sits there doing nothing.

```bash
helm upgrade --install ingress-nginx ingress-nginx \
  --repo https://kubernetes.github.io/ingress-nginx \
  --namespace ingress-nginx --create-namespace

kubectl get pods -n ingress-nginx
kubectl get svc -n ingress-nginx
```

- `helm upgrade --install` installs it, or upgrades it if it is already there.
- `ingress-nginx ingress-nginx` is the release name, then the chart name.
- `--repo` is the chart repository URL, so no `helm repo add` is needed.
- `--namespace` puts it in its own namespace. `--create-namespace` creates that namespace.

The controller runs as a Service of type `LoadBalancer`. On EKS that makes AWS create a real load balancer. When `EXTERNAL-IP` shows a hostname ending in `elb.amazonaws.com`, it is ready, and `kubectl get ing` shows the same hostname in `ADDRESS`. That load balancer is the public front door of the app.

The `CLASS` column in `kubectl get ing` says `nginx`. That ties the Ingress to this controller.

**Heads up: ingress-nginx is retired.** Kubernetes ended it in March 2026. Existing installs keep working, but there are no more bug fixes or security patches ([announcement](https://www.kubernetes.io/blog/2026/01/29/ingress-nginx-statement/)). I still used it because the course uses it and this cluster is for practice and gets deleted. Don't use it for anything real.

**Other options**

- AWS Load Balancer Controller: AWS's own. It creates an Application Load Balancer straight from the Ingress. It needs IAM and OIDC setup first. **Recommended on AWS.**
- A Gateway API controller (Envoy Gateway and others): the newer Kubernetes way, and what the Kubernetes team points to.
- Traefik: another Ingress controller.

### Issues I hit

> **Ingress `ADDRESS` stayed empty and ArgoCD showed the ingress as "Progressing"**
> - Cause: no ingress controller in the new cluster.
> - Fix: install one as above.
> - Avoid: after creating a cluster, run `kubectl get pods -n ingress-nginx` before debugging anything else.

---

## 8. Open the app

First test without touching your machine's settings:

```bash
kubectl get ing
curl -H "Host: go-web-app.local" http://<address-from-ADDRESS-column>/home
```

- `curl` makes an HTTP request.
- `-H "Host: ..."` sends an extra header. The Ingress rule matches on the host name, so the request has to carry `go-web-app.local`.

For the browser, map the name to the load balancer in `/etc/hosts`:

```bash
dig +short <address-from-ADDRESS-column>
echo "<one-of-the-ips>  go-web-app.local" | sudo tee -a /etc/hosts
sudo dscacheutil -flushcache; sudo killall -HUP mDNSResponder
```

- `dig +short` prints the IP addresses behind the load balancer's hostname. Pick one.
- `tee -a` appends to a file. `sudo tee` is used because `sudo echo ... >> /etc/hosts` doesn't work: the `>>` is done by your own shell, not by `sudo`.
- The last line clears the macOS DNS cache.

Open `http://go-web-app.local` with `http`, not `https`. There is no TLS here. Load balancer IPs can change, so if it stops working, run `dig` again. The hostname is the stable part.

**Other options.** A real domain with a Route 53 record pointing at the load balancer, plus a TLS certificate (ACM or cert-manager). **Recommended.** A hosts file entry is the leanest thing that works. On macOS `.local` names also go through mDNS and can be slow. `.test` is reserved for this kind of use.

### Issues I hit

> **`curl` to a node's public IP and NodePort timed out**
> - Cause: node ports are not open to the internet in the node security group, and there was no controller listening anyway.
> - Fix: use the load balancer hostname, never a node IP.
> - Avoid: the entry point is always the load balancer.

> **My hosts file pointed to a node IP**
> - Cause: I had the wrong IP in there, and old entries from deleted clusters stay until you remove them.
> - Fix: `dig` the load balancer, edit `/etc/hosts`, delete stale lines.

> **`sudo echo "..." >> /etc/hosts` gave "permission denied"**
> - Cause: the redirect runs as your user.
> - Fix: `| sudo tee -a /etc/hosts`, or `sudo vim /etc/hosts`.

> **`nslookup go-web-app.local` said NXDOMAIN even with the hosts entry**
> - Cause: `nslookup` only asks DNS servers and skips `/etc/hosts`.
> - Fix: test with `ping go-web-app.local` or `curl`.

Debug order when the app doesn't open:

1. `kubectl get pods -n ingress-nginx`: is a controller running?
2. `kubectl get ing`: does it have an `ADDRESS`?
3. `curl -H "Host: ..." http://<address>`: does the app answer through the load balancer?
4. If 3 works and the browser doesn't, it is the hosts file or DNS.

---

## 9. Helm

`helm/go-web-app-chart/` holds the same deployment, service and ingress as `k8s/manifest/`, but as templates with variables. The variable CI changes is the image tag in `values.yaml`.

You don't need to run Helm yourself, because ArgoCD renders the chart. To see Helm on its own:

```bash
helm install go-web-app ./helm/go-web-app-chart
helm list
helm uninstall go-web-app
```

- `helm install <release-name> <chart-path>` deploys the chart as a named release.
- `helm list` shows releases.
- `helm uninstall` removes the release. Do this before ArgoCD takes over, same reason as step 6.

**Why Helm.** The course uses it, it is the most common packaging format you will meet, and CI only has to change one line (`tag:`). **Other options:** Kustomize (built into `kubectl apply -k`) or plain manifests with `sed`. For one app those are leaner. Helm earns its place when you have several environments or want to install other people's charts, like ingress-nginx above.

---

## 10. ArgoCD

```bash
kubectl create namespace argocd
kubectl apply --server-side --force-conflicts -n argocd \
  -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
kubectl get pods -n argocd
```

- `kubectl create namespace argocd` makes a namespace to keep ArgoCD apart from your app.
- `-n argocd` runs the command in that namespace.
- `--server-side` lets the Kubernetes API server merge the YAML. A normal `apply` stores a copy of the whole file in an annotation, and one ArgoCD resource is too big for that.
- `--force-conflicts` takes ownership of fields if there is a conflict.
- `-f <url>` can read the file straight from a URL.
- `stable` moves over time. Pin a version tag if you want the same install every time.

All pods should end up `Running`.

### Open the UI and log in

Leanest that works:

```bash
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

`port-forward` opens a tunnel from your laptop to the Service. `svc/argocd-server` is the target, `8080:443` is `local-port:service-port`. Open `https://localhost:8080` and accept the certificate warning (ArgoCD makes its own certificate). Nothing is exposed to the internet.

What I did instead:

```bash
kubectl patch svc argocd-server -n argocd -p '{"spec": {"type": "LoadBalancer"}}'
kubectl get svc argocd-server -n argocd
```

`patch -p` changes part of an object, here the Service type, so AWS creates a load balancer for it. It works, but it costs a second load balancer and puts the login page on the public internet.

Password for the `admin` user:

```bash
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d; echo
```

- `-o jsonpath="{.data.password}"` prints only the `password` field of the secret.
- Secrets are stored base64 encoded, `base64 -d` decodes it.
- `echo` adds the missing line break.

Change the password in the UI (User Info, Update Password), then delete the initial secret: `kubectl delete secret argocd-initial-admin-secret -n argocd`.

### Create the app

In the UI (New App) or with a YAML file and `kubectl apply -f`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-portfolio
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/Zamio-dev/my-portfolio.git
    targetRevision: HEAD
    path: helm/go-web-app-chart
  destination:
    server: https://kubernetes.default.svc
    namespace: default
  syncPolicy:
    automated: {}
```

- `repoURL`, `targetRevision`, `path`: which repo, which branch or commit (`HEAD` is the default branch) and which folder to watch. Public repo, so ArgoCD needs no credentials.
- `destination.server`: `https://kubernetes.default.svc` means "the cluster ArgoCD itself runs in".
- `destination.namespace`: where the app goes.
- `automated: {}`: sync on its own when Git changes. ArgoCD checks Git about every 3 minutes, so a deploy is not instant. **Recommended:** `automated: {prune: true, selfHeal: true}`. `prune` deletes objects you removed from Git. `selfHeal` undoes manual changes in the cluster.

**ArgoCD runs inside the cluster.** Port-forward or a load balancer only decide how your browser reaches the UI. Your laptop is not part of the deploy. Close it and ArgoCD keeps syncing. ArgoCD manages Kubernetes objects, not AWS itself. It doesn't create clusters, VPCs or servers. The only AWS thing it triggers is the load balancer behind a `LoadBalancer` Service.

**Why ArgoCD.** The cluster pulls from Git, so GitHub never holds cluster credentials, Git is the source of truth, ArgoCD shows drift, and a rollback is a revert commit. **Other options:** Flux does the same without the UI. The leanest option is no GitOps tool at all: CI runs `helm upgrade` itself, which needs a kubeconfig stored as a GitHub secret.

### Issues I hit

> **`applicationsets.argoproj.io is invalid: metadata.annotations: Too long: may not be more than 262144 bytes`, then `argocd-applicationset-controller` in `CrashLoopBackOff`**
> - Cause: plain `kubectl apply` created everything except that one big CRD. The controller crashed because the CRD didn't exist.
> - Fix: re-run the install with `--server-side --force-conflicts`. The pod recovered after that. If you don't want to wait: `kubectl rollout restart deployment argocd-applicationset-controller -n argocd`.
> - Avoid: use `--server-side` from the start.

> **`kubectl patch` failed with `invalid character '\\' looking for beginning of object key`**
> - Cause: I pasted the JSON with backslash escaped quotes.
> - Fix: use the plain version shown above, single quotes outside, double quotes inside.

> **"No resources found" and "NotFound" when looking for ArgoCD objects**
> - Cause: ArgoCD lives in the `argocd` namespace. `kubectl` looks in `default` unless you add `-n`. I also typed `-n argo` once.
> - Fix: add `-n argocd`.

> **Could not open the ArgoCD UI on a node IP**
> - Cause: same as step 8. Node ports are closed.
> - Fix: port-forward, or the `EXTERNAL-IP` hostname of the Service. Use `https`. `curl` needs `-k` because of the self made certificate.

---

## 11. Teardown

```bash
kubectl delete svc argocd-server -n argocd
helm uninstall ingress-nginx -n ingress-nginx
eksctl delete cluster --name demo-cluster --region ap-southeast-2
```

- The first line is only needed if you patched ArgoCD to `LoadBalancer`. Deleting the Service removes its AWS load balancer.
- The second removes the ingress controller and its load balancer.
- `eksctl delete cluster` removes the nodes, the control plane and the VPC.

Delete the load balancer Services first. A load balancer that Kubernetes created is not part of the eksctl stacks, and a leftover one can block the VPC from being deleted and keeps billing. Afterwards check EC2, Load Balancers in the AWS console. The list should be empty.

Also remove the `go-web-app.local` line from `/etc/hosts`.

---

## Credits

Course and original app: [iam-veeramalla/go-web-app-devops](https://github.com/iam-veeramalla/go-web-app-devops). Licensed under Apache-2.0, see `LICENSE`.
