import { render } from '@testing-library/react';
import { MassActionDetail } from 'src/sections/mass-actions/mass-action-detail';
import { ScriptHistoryDialog } from 'src/sections/scripts/script-history-dialog';

const httpRequest = jest.fn(() => new Promise(() => {}));

jest.mock('src/contexts/backend-context', () => ({
  useBackendContext: () => ({
    apiPrefix: '/api/tenants/test',
    httpRequest,
  }),
}));

describe('hook dependency null safety', () => {
  beforeEach(() => {
    httpRequest.mockClear();
  });

  test('mass action detail renders before the action response exists', () => {
    expect(() => render(<MassActionDetail actionId="action-1" />)).not.toThrow();
  });

  test('script history dialog renders while no script is selected', () => {
    expect(() => render(
      <ScriptHistoryDialog open={false} onClose={jest.fn()} script={null} />
    )).not.toThrow();
  });
});
