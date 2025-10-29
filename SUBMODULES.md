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

## Cheat sheet


```makefile
make init-submodules
make update-submodules
```


| Command | Description |
|---------|-------------|
| `git submodule update --init` | Initialize and update submodules |
| `git submodule update --remote` | Update submodules to latest remote commits |
| `git submodule status` | Check status of all submodules |
| `git submodule foreach git pull origin main` | Pull latest changes in all submodules |
| `git diff --submodule` | See detailed changes in submodules |


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

