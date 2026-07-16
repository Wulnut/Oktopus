export const uploadWhitelistBatchCSV = (httpRequest, apiPrefix, file) => (
  httpRequest(
    `${apiPrefix}/lock/whitelist/batch`,
    'POST',
    file,
    { 'Content-Type': 'text/csv' },
    'json'
  )
);
