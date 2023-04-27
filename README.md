# chatgpt-create-unit-tests

A GitHub Action that analyses a Pull Request and adds unit tests if necessary / possible

## Usage

Minimal example to put into your action YAML file:

```
name: Suggest tests for new functions

on: [pull_request]

jobs:
  build:
    name: Suggest tests for new functions
    runs-on: ubuntu-latest
    steps:
    - name: Check out code
      uses: actions/checkout@v3
      with:
        fetch-depth: 0
    - name: Create patch file & store in workspace
      run: git diff origin/${GITHUB_BASE_REF} origin/${GITHUB_HEAD_REF} &> ${GITHUB_WORKSPACE}/patch
    - name: Ask ChatGPT for unit tests for new functions
      uses: zebroc/chatgpt-create-unit-tests@master
      env:
        OPENAI_TOKEN: ${{ secrets.OPENAI_TOKEN }}
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

The important parts are the "with: fetch-depth: 0" option for the checkout action so that he entire history is there and
providing your OpenAI token as a secret called "OPENAI_TOKEN". GITHUB_TOKEN is automatically provided by GitHub, it
just needs the right permissions:

To allow the comment on PRs, you need to go to Settings --> Actions --> General --> Workflow permissions
and select the option "Read and write permissions". This action will not fail if you don't do this, but
no commenting will happen.

If you want more output, you can set a DEBUG environment variable, like so:

```
…
    - name: Ask ChatGPT for unit tests for new functions
      uses: zebroc/chatgpt-create-unit-tests@master
      env:
        OPENAI_TOKEN: ${{ secrets.OPENAI_TOKEN }}
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        DEBUG: true
```

## Inputs

An example using custom prompts and a very small max patch size:

```
name: Code review & security review

on: [pull_request]

jobs:
  build:
    name: Suggest tests for new functions
    runs-on: ubuntu-latest
    steps:
    - name: Check out code
      uses: actions/checkout@v3
      with:
        fetch-depth: 0
    - name: Create patch file & store in workspace
      run: git diff origin/${GITHUB_BASE_REF} origin/${GITHUB_HEAD_REF} &> ${GITHUB_WORKSPACE}/patch
    - name: Ask ChatGPT for a code and security review
      uses: zebroc/chatgpt-create-unit-tests@master
      with:
        prompts: '{"code":"Do a code review for this:\n\n%s","sec":"Do a security review for this:\n\n%s"}'
        maxpatchsize: '50'
      env:
        OPENAI_TOKEN: ${{ secrets.OPENAI_TOKEN }}
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Custom prompts

You can use inputs to provide the prompts that you want to use. They go into the "prompts" input in the form of a JSON
object which is marshalled into a map[string]string, example:

```json
{
  "codereview": "Please do a code review for this patch:\n\n%s",
  "security": "Are there any security issues this patch:\n\n%s"
}
```

A prompt has exactly one placeholder (the %s). That is the content of the patch / diff.

### Max patch size

Only ask ChatGPT if your patch is below this size: Serves both as a reminder to keep PRs small and also limits requests
to ChatGPT that are too large anyways.
