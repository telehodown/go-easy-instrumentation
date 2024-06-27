<a href="https://opensource.newrelic.com/oss-category/#new-relic-experimental"><picture><source media="(prefers-color-scheme: dark)" srcset="https://github.com/newrelic/opensource-website/raw/main/src/images/categories/dark/Experimental.png"><source media="(prefers-color-scheme: light)" srcset="https://github.com/newrelic/opensource-website/raw/main/src/images/categories/Experimental.png"><img alt="New Relic Open Source experimental project banner." src="https://github.com/newrelic/opensource-website/raw/main/src/images/categories/Experimental.png"></picture></a>

# Go Agent Assisted Instrumentation
Go is a compiled language with an opaque runtime, making it unable to support automatic instrumentation the way other languages are, which is why the New Relic Go Agent is designed as an SDK. That makes installing and getting the most out of the Go Agent something that requires much more upfront investment than other language agents. To address this, the Go Agent team created the Go Agent Assisted Instrumentation tool, which will do most of the instrumentation work for you by generating changes to your source code that instruments your application with the New Relic Go Agent.

## Alpha Preview Notice
This is an early pre-release preview of the Assisted Instrumentation product. As such, it is still under active development. We appreciate any feedback you may have about any issues you find as you explore this tool.

Since this is an early proof of concept, the scope of what it tries to instrument in your application is limited to the listed features and supports capturing telemetry from the Go standard library, and net/http library.

 - Wrapping net/http handle functions
 - Wrapping net/http mux handle functions
 - Creating external segments for, and injecting distributed tracing into, any external call made with a net/http Request object
 - Injecting a roundtripper into any net/http client
 - Capturing errors in any function wrapped or traced by a transaction
 - Tracing locally defined functions that are invoked in the application's main() method with a transaction
 - Tracing async functions and function literals with an async segment


Regardless of the scope, this tool will not interfere with your application's operation, so it doesn't make any changes to your code directly. Instead, it analyzes your source code, identifies opportunities to instrument it, then suggests changes to your code that use the New Relic Go Agent SDK to capture telemetry data. These additions will be in the form of a `.diff` file, which you should review before applying to your source code.

As part of the analysis, this tool may invoke `go get` or other Go language toolchain commands which may modify your `go.mod` file, but not your actual source code.

## Installation
Please have a version of Go installed that is within the support window for the current [Go programming language lifecycle](https://endoflife.date/go).
1. Clone this repository to a directory on your system. For example: `git clone .../go-agent-pre-instrumentation`
2. Go into that directory: `cd go-agent-pre-instrumentation`
3. Resolve any third-party dependencies: `go mod tidy`

## Getting Started
This tool works best when used with git. It's a best practice to ensure that your application is on a branch without any unstaged changes before applying any of the generated changes to it. After checking that, follow these steps to generate and apply the changes that install the New Relic Go Agent in an application:

1. go to parser directory: ```cd parser```
2. run the CLI tool: ```go run .```
3. follow the prompts to customize the process as needed
4. open your instrumented application directory: ```cd ../my-application```
5. There will be a `.diff` file written there. By default, this diff file will be named `new-relic-instrumentation.diff`. Verify that the contents of this diff file are complete and corect before applying it to any code.
6. Apply the changes using `git apply`

Once the changes are applied, the application should run with the New Relic Go Agent installed. If the agent installation is not working the way you want it to, the changes can easily be undone using git tools by either stashing them with `git stash` or reverting the code to a previous commit.

## Support
This is an experimental product, and New Relic is not offering official support at the moment. Please create issues in github if you are encountering a problem that you're unable to resolve. When creating issues, its vital to include as much of the prompted for information as possible. This enables us to get to the root cause of the issue much more quickly. Please also make sure to search existing issues before creating a new one.

## Contributing
We encourage your contributions to improve the Go Agent Assisted Instrumentation tool! Keep in mind when you submit your pull request, you'll need to sign the CLA via the click-through using CLA-Assistant. You only have to sign the CLA one time per project.
If you have any questions, or to execute our corporate CLA, required if your contribution is on behalf of a company,  please drop us an email at opensource@newrelic.com.


## License
Go Agent Assisted Instrumentation is licensed under the [Apache 2.0](http://apache.org/licenses/LICENSE-2.0.txt) License.
>The Go Agent Assisted Instrumentation tool also uses source code from third-party libraries. You can find full details on which libraries are used and the terms under which they are licensed in the third-party notices document.
