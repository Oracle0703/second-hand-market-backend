#!/usr/bin/env node

import fs from 'node:fs'

const [reportPath, ...requirements] = process.argv.slice(2)
if (!reportPath || requirements.length === 0) {
  throw new Error('usage: validate-vitest-report.mjs <report.json> <file-suffix::full-test-name>...')
}

const report = JSON.parse(fs.readFileSync(reportPath, 'utf8'))
const zeroFields = [
  'numFailedTestSuites',
  'numPendingTestSuites',
  'numFailedTests',
  'numPendingTests',
  'numTodoTests'
]

if (report.success !== true ||
    !Number.isInteger(report.numTotalTests) ||
    report.numTotalTests <= 0 ||
    report.numPassedTests !== report.numTotalTests ||
    zeroFields.some((field) => report[field] !== 0)) {
  throw new Error(`Vitest report is not fully green: ${JSON.stringify({
    success: report.success,
    totalSuites: report.numTotalTestSuites,
    passedSuites: report.numPassedTestSuites,
    failedSuites: report.numFailedTestSuites,
    pendingSuites: report.numPendingTestSuites,
    totalTests: report.numTotalTests,
    passedTests: report.numPassedTests,
    failedTests: report.numFailedTests,
    pendingTests: report.numPendingTests,
    todoTests: report.numTodoTests
  })}`)
}

const results = Array.isArray(report.testResults) ? report.testResults : []
for (const result of results) {
  if (result.status !== 'passed') {
    throw new Error(`test file did not pass: ${result.name} (${result.status})`)
  }
  for (const assertion of result.assertionResults ?? []) {
    if (assertion.status !== 'passed') {
      throw new Error(`test assertion did not pass: ${assertion.fullName} (${assertion.status})`)
    }
  }
}

for (const requirement of requirements) {
  const separator = requirement.indexOf('::')
  if (separator <= 0) {
    throw new Error(`invalid required test descriptor: ${requirement}`)
  }
  const fileSuffix = requirement.slice(0, separator).replaceAll('\\', '/')
  const fullName = requirement.slice(separator + 2)
  const fileResult = results.find((result) =>
    String(result.name ?? '').replaceAll('\\', '/').endsWith(fileSuffix)
  )
  if (!fileResult) {
    throw new Error(`required test file was not reported: ${fileSuffix}`)
  }
  const assertion = (fileResult.assertionResults ?? []).find((item) =>
    item.fullName === fullName && item.status === 'passed'
  )
  if (!assertion) {
    throw new Error(`required test did not report passed: ${fileSuffix} :: ${fullName}`)
  }
}

console.log(`validated_vitest_tests=${report.numPassedTests}`)
