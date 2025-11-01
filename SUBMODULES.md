# JAM Submodules 

This documents using the `services` and `pvm` as Git submodule in the JAM project:

- services: https://github.com/jam-duna/services/
- pvm: https://github.com/colorfulnotion/jam/issues

Submodules can support different JAM implementations working with our EVM/.. services + PVM recompiler.

### For Fresh Clones

When cloning this repository fresh, initialize submodules:
```bash
git clone git@github.com:colorfulnotion/jam.git
cd jam
git submodule update --init --recursive
```

Or clone with submodules in one command:
```bash
git clone --recurse-submodules git@github.com:colorfulnotion/jam.git
```

### Updating the Services Submodule
To get the latest changes from the services repository:
```bash
# Update to the latest commit on the main branch
git submodule update --remote services

# Commit the updated submodule reference
git add services
git commit -m "Update services submodule to latest version"
```

### Making Changes to Services
If you need to make changes to the services:

1. **Navigate to the submodule directory:**
   ```bash
   cd services
   ```

2. **Create a new branch for your changes:**
   ```bash
   git checkout -b your-feature-name
   ```

3. **Make your changes and commit:**
   ```bash
   # Make your changes
   git add .
   git commit -m "Your change description"
   ```

4. **Push to the services repository:**
   ```bash
   git push origin your-feature-name
   ```

5. **Create a pull request in the jam-duna/services repository**

6. **After PR is merged, update the submodule reference in the main repo:**
   ```bash
   cd ..  # Back to jam root
   git submodule update --remote services
   git add services
   git commit -m "Update services submodule with new changes"
   ```

## Quick Update Commands

### Using Makefile (Recommended)

The easiest way to update a submodule:

```bash
# Update pvm submodule to latest commit from local ../pvm repo
make update-pvm-submodule

# Update pvm submodule to a specific commit
make update-pvm-submodule COMMIT=456513b

# Update services submodule to latest commit from local ../services repo
make update-services-submodule

# Update services submodule to a specific commit
make update-services-submodule COMMIT=abc123

# Then commit and push
git commit -m "Update pvm submodule to 456513b"
git push origin <your-branch>
```

**What this does:**
1. Fetches latest changes from the submodule remote
2. If COMMIT is specified: checks out that commit
3. If COMMIT is omitted: uses latest commit from local ../pvm (or ../services) directory
4. Stages the submodule pointer change
5. Shows you next steps

### Manual Method

If you prefer to do it manually:

```bash
# Step 1: Navigate into the submodule
cd pvm  # or cd services

# Step 2: Fetch latest changes
git fetch origin

# Step 3: Checkout specific commit
git checkout 456513b

# Step 4: Go back to jam repo root
cd ..

# Step 5: Stage the submodule pointer change
git add pvm  # or git add services

# Step 6: Commit
git commit -m "Update pvm submodule to 456513b"

# Step 7: Push
git push origin <your-branch>
```

## Cheat Sheet

```makefile
# Initialize submodules (first time)
make init-submodules

# Update all submodules to latest on their tracked branches
make update-submodules

# Update pvm submodule to latest from local ../pvm (COMMIT optional)
make update-pvm-submodule

# Update pvm submodule to specific commit
make update-pvm-submodule COMMIT=<hash>

# Update services submodule to latest from local ../services (COMMIT optional)
make update-services-submodule

# Update services submodule to specific commit
make update-services-submodule COMMIT=<hash>
```

| Command | Description |
|---------|-------------|
| `git submodule update --init` | Initialize and update submodules |
| `git submodule update --remote` | Update submodules to latest remote commits |
| `git submodule status` | Check status of all submodules |
| `git submodule foreach git pull origin main` | Pull latest changes in all submodules |
| `git diff --submodule` | See detailed changes in submodules |
| `make update-pvm-submodule` | Update pvm to latest from local ../pvm (stages change) |
| `make update-pvm-submodule COMMIT=<hash>` | Update pvm to specific commit (stages change) |
| `make update-services-submodule` | Update services to latest from local ../services (stages change) |
| `make update-services-submodule COMMIT=<hash>` | Update services to specific commit (stages change) |


### Submodule not initialized
If you see an empty `services/` directory:
```bash
git submodule update --init --recursive
```

### Detached HEAD in submodule
This is normal for submodules. To make changes:
```bash
cd services
git checkout main  # or appropriate branch
```

### Merge conflicts in submodule reference
When merging branches with different submodule commits:
```bash
# Choose which version to use
git checkout --theirs services  # Use their version
# OR
git checkout --ours services    # Use our version

# Then update the submodule
git submodule update --init
git add services
git commit
```

## Important!

1. **Submodule commits are references**: The main repository stores a reference to a specific commit in the submodule, not the actual files.

2. **Always commit submodule changes first**: When making changes in services/, commit and push those changes to the services repository before updating the reference in the main repository.

3. **Team coordination**: Ensure all team members understand submodule workflow to avoid confusion and conflicts.

4. **Version pinning**: The submodule reference pins to a specific commit, providing version stability. Update deliberately when needed.

## Additional Resources

- [Git Submodules Documentation](https://git-scm.com/book/en/v2/Git-Tools-Submodules)
- [jam-duna/services Repository](https://github.com/jam-duna/services)
- [GitHub: Working with Submodules](https://github.blog/2016-02-01-working-with-submodules/)

