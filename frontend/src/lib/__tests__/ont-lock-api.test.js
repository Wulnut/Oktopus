import { uploadWhitelistBatchCSV } from '../ont-lock-api';

test('uploads whitelist CSV through BackendContext httpRequest', async () => {
  const httpRequest = jest.fn().mockResolvedValue({ status: 202, result: { created: 1 } });
  const file = new File(['sn,allowed_ip_range\nSN-1,10.0.0.1/32'], 'policies.csv', {
    type: 'text/csv',
  });

  const response = await uploadWhitelistBatchCSV(httpRequest, '/api/tenants/test', file);

  expect(httpRequest).toHaveBeenCalledWith(
    '/api/tenants/test/lock/whitelist/batch',
    'POST',
    file,
    { 'Content-Type': 'text/csv' },
    'json'
  );
  expect(response).toEqual({ status: 202, result: { created: 1 } });
});
