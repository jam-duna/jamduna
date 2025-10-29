# Git Submodules Migration Guide for JAM

## Overview
This document provides instructions for converting the `services/` directory into a Git submodule using the same services repository that is used in the majik project. This allows for shared service implementations across different JAM implementations while maintaining independent versioning.

## Objectives
1. Replace the local `services/` directory with the jam-duna/services submodule
2. Maintain consistency with the majik repository's service implementations
3. Enable independent development and versioning of services
4. Allow easy updates from the upstream services repository

## Prerequisites
- Ensure all local changes in `services/` are committed or backed up
- Have write access to your jam repository
- Git version 2.13 or higher (for better submodule support)

## Migration Steps

### Step 1: Backup Current Services (Optional but Recommended)
```bash
# Create a backup branch with current services
cd /Users/sourabhniyogi/Documents/jam
git checkout -b backup-services-before-submodule
git add .
git commit -m "Backup: services directory before submodule migration"
git checkout main  # or your main branch
```

### Step 2: Remove Existing Services Directory
```bash
# Remove the services directory from git tracking
git rm -r services/
git commit -m "Remove services directory to prepare for submodule"
```

### Step 3: Add Services as Submodule
```bash
# Add the jam-duna/services repository as a submodule
git submodule add git@github.com:jam-duna/services.git services
git commit -m "Add services as submodule from jam-duna/services"
```

### Step 4: Initialize and Update Submodule
```bash
# Initialize and fetch the submodule content
git submodule update --init --recursive
```

### Step 5: Push Changes
```bash
# Push the changes to your repository
git push origin main  # or your branch name
```

## Working with Submodules

### For Fresh Clones
When someone clones your repository fresh, they need to initialize submodules:
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
   git checkout -b feature/your-feature-name
   ```

3. **Make your changes and commit:**
   ```bash
   # Make your changes
   git add .
   git commit -m "Your change description"
   ```

4. **Push to the services repository:**
   ```bash
   git push origin feature/your-feature-name
   ```

5. **Create a pull request in the jam-duna/services repository**

6. **After PR is merged, update the submodule reference in the main repo:**
   ```bash
   cd ..  # Back to jam root
   git submodule update --remote services
   git add services
   git commit -m "Update services submodule with new changes"
   ```

## Build System Updates

### Update Makefile (if needed)
Add these targets to your Makefile for convenient submodule management:

```makefile
.PHONY: init-submodules update-submodules

init-submodules:
	git submodule update --init --recursive

update-submodules:
	git submodule update --remote --merge

# Add to your existing build target
build: init-submodules
	# Your existing build commands
```

### Update CI/CD Pipeline
Ensure your CI/CD pipeline initializes submodules:

**GitHub Actions example:**
```yaml
- name: Checkout code
  uses: actions/checkout@v3
  with:
    submodules: recursive
```

**Or manually:**
```yaml
- name: Checkout code
  uses: actions/checkout@v3
- name: Initialize submodules
  run: git submodule update --init --recursive
```

## Common Commands Reference

| Command | Description |
|---------|-------------|
| `git submodule status` | Check status of all submodules |
| `git submodule update --init` | Initialize and update submodules |
| `git submodule update --remote` | Update submodules to latest remote commits |
| `git submodule foreach git pull origin main` | Pull latest changes in all submodules |
| `git diff --submodule` | See detailed changes in submodules |

## Troubleshooting

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

### Authentication issues
If you encounter authentication issues with SSH:
1. Ensure your SSH key is added to GitHub
2. Test connection: `ssh -T git@github.com`
3. If using HTTPS, you may need to configure credentials

## Important Notes

1. **Submodule commits are references**: The main repository stores a reference to a specific commit in the submodule, not the actual files.

2. **Always commit submodule changes first**: When making changes in services/, commit and push those changes to the services repository before updating the reference in the main repository.

3. **Team coordination**: Ensure all team members understand submodule workflow to avoid confusion and conflicts.

4. **Version pinning**: The submodule reference pins to a specific commit, providing version stability. Update deliberately when needed.

## Additional Resources

- [Git Submodules Documentation](https://git-scm.com/book/en/v2/Git-Tools-Submodules)
- [jam-duna/services Repository](https://github.com/jam-duna/services)
- [GitHub: Working with Submodules](https://github.blog/2016-02-01-working-with-submodules/)

## Contact

For issues or questions about the services submodule, please open an issue in the appropriate repository:
- Services implementation: https://github.com/jam-duna/services/issues
- JAM integration: https://github.com/colorfulnotion/jam/issues