import { useEffect, useState } from 'react';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  Grid,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  SvgIcon,
  Switch,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import PlusIcon from '@heroicons/react/24/solid/PlusIcon';
import TrashIcon from '@heroicons/react/24/solid/TrashIcon';
import ChevronUpIcon from '@heroicons/react/24/solid/ChevronUpIcon';
import ChevronDownIcon from '@heroicons/react/24/solid/ChevronDownIcon';
import ArrowsUpDownIcon from '@heroicons/react/24/solid/ArrowsUpDownIcon';

const STEP_TYPES = ['GET', 'SET', 'ADD', 'DELETE', 'OPERATE', 'CONDITION', 'DELAY'];
const ERROR_POLICIES = ['abort', 'continue'];
const OPERATORS = ['==', '!=', '>', '<', 'contains', 'exists'];

const newStep = () => ({
  id: 'step_' + Date.now() + '_' + Math.random().toString(36).slice(2, 6),
  name: '',
  type: 'GET',
  on_error: 'abort',
  param_paths: [''],
  max_depth: 0,
  save_result_as: '',
  obj_path: '',
  param_settings: [],
  obj_paths: [''],
  command: '',
  command_key: '',
  input_args: [],
  condition: { source: '', path: '', param: '', operator: '==', value: '' },
  on_true: '',
  on_false: '',
  duration_ms: 1000,
});

const newVariable = () => ({
  name: '',
  description: '',
  default: '',
  required: false,
});

const emptyScript = () => ({
  name: '',
  description: '',
  tags: [],
  variables: [],
  steps: [newStep()],
});

export const ScriptEditorDialog = ({ open, onClose, onSave, script }) => {
  const [form, setForm] = useState(emptyScript());
  const [tagInput, setTagInput] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open) {
      if (script) {
        setForm({
          name: script.name || '',
          description: script.description || '',
          tags: script.tags || [],
          variables: (script.variables || []).map((v) => ({ ...v })),
          steps: (script.steps || []).map((s) => ({
            ...s,
            param_paths: s.param_paths?.length ? [...s.param_paths] : [''],
            obj_paths: s.obj_paths?.length ? [...s.obj_paths] : [''],
            param_settings: (s.param_settings || []).map((ps) => ({ ...ps })),
            input_args: (s.input_args || []).map((ia) => ({ ...ia })),
            condition: s.condition ? { ...s.condition } : { source: '', path: '', param: '', operator: '==', value: '' },
          })),
        });
      } else {
        setForm(emptyScript());
      }
    }
  }, [open, script]);

  const updateStep = (index, field, value) => {
    setForm((prev) => {
      const steps = [...prev.steps];
      steps[index] = { ...steps[index], [field]: value };
      return { ...prev, steps };
    });
  };

  const addStep = () => {
    setForm((prev) => ({ ...prev, steps: [...prev.steps, newStep()] }));
  };

  const removeStep = (index) => {
    setForm((prev) => ({ ...prev, steps: prev.steps.filter((_, i) => i !== index) }));
  };

  const moveStep = (index, direction) => {
    setForm((prev) => {
      const steps = [...prev.steps];
      const target = index + direction;
      if (target < 0 || target >= steps.length) return prev;
      [steps[index], steps[target]] = [steps[target], steps[index]];
      return { ...prev, steps };
    });
  };

  const addVariable = () => {
    setForm((prev) => ({ ...prev, variables: [...prev.variables, newVariable()] }));
  };

  const updateVariable = (index, field, value) => {
    setForm((prev) => {
      const variables = [...prev.variables];
      variables[index] = { ...variables[index], [field]: value };
      return { ...prev, variables };
    });
  };

  const removeVariable = (index) => {
    setForm((prev) => ({ ...prev, variables: prev.variables.filter((_, i) => i !== index) }));
  };

  const addTag = () => {
    const tag = tagInput.trim();
    if (tag && !form.tags.includes(tag)) {
      setForm((prev) => ({ ...prev, tags: [...prev.tags, tag] }));
    }
    setTagInput('');
  };

  const removeTag = (tag) => {
    setForm((prev) => ({ ...prev, tags: prev.tags.filter((t) => t !== tag) }));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave(form);
    } finally {
      setSaving(false);
    }
  };

  const stepOptions = form.steps.map((s) => ({ id: s.id, name: s.name }));

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth scroll="paper">
      <DialogTitle>{script?.id ? 'Edit Script' : 'New Script'}</DialogTitle>
      <DialogContent dividers sx={{ pt: 3 }}>
        <Stack spacing={3}>
          {/* Basic info */}
          <TextField
            label="Name"
            value={form.name}
            onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
            fullWidth
            required
            id="script-name"
            autoComplete="off"
          />
          <TextField
            label="Description"
            value={form.description}
            onChange={(e) => setForm((prev) => ({ ...prev, description: e.target.value }))}
            fullWidth
            multiline
            rows={2}
            id="script-description"
            autoComplete="off"
          />

          {/* Tags */}
          <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap alignItems="center">
            {form.tags.map((tag) => (
              <Chip key={tag} label={tag} onDelete={() => removeTag(tag)} size="small" />
            ))}
            <TextField
              label="Tags"
              placeholder="Press Enter to add"
              value={tagInput}
              onChange={(e) => setTagInput(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addTag(); } }}
              sx={{ width: 180 }}
              id="script-tag-input"
              autoComplete="off"
            />
          </Stack>

          <Divider />

          {/* Variables */}
          <Box>
            <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1 }}>
              <Typography variant="subtitle1">Variables</Typography>
              <Button size="small" startIcon={<SvgIcon fontSize="small"><PlusIcon /></SvgIcon>} onClick={addVariable}>
                Add Variable
              </Button>
            </Stack>
            <Stack spacing={1.5}>
              {form.variables.map((v, vi) => (
                <Stack key={vi} direction="row" spacing={1.5} alignItems="center">
                  <TextField
                    label="Name"
                    value={v.name}
                    onChange={(e) => updateVariable(vi, 'name', e.target.value)}
                    sx={{ width: 160 }}
                    id={`var-name-${vi}`}
                    autoComplete="off"
                  />
                  <TextField
                    label="Description"
                    value={v.description}
                    onChange={(e) => updateVariable(vi, 'description', e.target.value)}
                    sx={{ flex: 1 }}
                    id={`var-desc-${vi}`}
                    autoComplete="off"
                  />
                  <TextField
                    label="Default"
                    value={v.default}
                    onChange={(e) => updateVariable(vi, 'default', e.target.value)}
                    sx={{ width: 140 }}
                    id={`var-default-${vi}`}
                    autoComplete="off"
                  />
                  <Tooltip title="Required">
                    <Switch
                      checked={v.required}
                      onChange={(e) => updateVariable(vi, 'required', e.target.checked)}
                    />
                  </Tooltip>
                  <IconButton onClick={() => removeVariable(vi)}>
                    <SvgIcon fontSize="small"><TrashIcon /></SvgIcon>
                  </IconButton>
                </Stack>
              ))}
            </Stack>
          </Box>

          <Divider />

          {/* Steps */}
          <Box>
            <Typography variant="subtitle1" sx={{ mb: 1 }}>Steps ({form.steps.length})</Typography>

            {form.steps.map((step, si) => (
              <Accordion key={step.id} defaultExpanded={form.steps.length === 1} sx={{ mb: 1 }}>
                <AccordionSummary expandIcon={<SvgIcon fontSize="small"><ArrowsUpDownIcon /></SvgIcon>}>
                  <Stack direction="row" spacing={1} alignItems="center" sx={{ width: '100%', mr: 1 }}>
                    <Chip label={si + 1} size="small" />
                    <Chip label={step.type} size="small" color="primary" variant="outlined" />
                    <Typography variant="body2" noWrap sx={{ flex: 1 }}>
                      {step.name || '(unnamed)'}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">{step.id}</Typography>
                  </Stack>
                </AccordionSummary>
                <AccordionDetails>
                  <StepEditor
                    step={step}
                    index={si}
                    totalSteps={form.steps.length}
                    stepOptions={stepOptions}
                    onChange={updateStep}
                    onRemove={removeStep}
                    onMove={moveStep}
                  />
                </AccordionDetails>
              </Accordion>
            ))}

            <Button
              fullWidth
              variant="outlined"
              startIcon={<SvgIcon fontSize="small"><PlusIcon /></SvgIcon>}
              onClick={addStep}
              sx={{ mt: 1 }}
            >
              Add Step
            </Button>
          </Box>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>Cancel</Button>
        <Button
          onClick={handleSave}
          variant="contained"
          disabled={saving || !form.name || form.steps.length === 0}
        >
          {saving ? 'Saving...' : 'Save'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

const stepLabel = (opt) => opt.name || opt.id;

const StepEditor = ({ step, index, totalSteps, stepOptions, onChange, onRemove, onMove }) => {
  const stepIds = stepOptions.map((s) => s.id);
  const update = (field, value) => onChange(index, field, value);

  const updateParamPath = (pi, value) => {
    const paths = [...(step.param_paths || [''])];
    paths[pi] = value;
    update('param_paths', paths);
  };

  const addParamPath = () => update('param_paths', [...(step.param_paths || []), '']);
  const removeParamPath = (pi) => update('param_paths', (step.param_paths || []).filter((_, i) => i !== pi));

  const updateParamSetting = (pi, field, value) => {
    const settings = [...(step.param_settings || [])];
    settings[pi] = { ...settings[pi], [field]: value };
    update('param_settings', settings);
  };

  const addParamSetting = () => update('param_settings', [...(step.param_settings || []), { param: '', value: '', required: false }]);
  const removeParamSetting = (pi) => update('param_settings', (step.param_settings || []).filter((_, i) => i !== pi));

  const updateInputArg = (ai, field, value) => {
    const args = [...(step.input_args || [])];
    args[ai] = { ...args[ai], [field]: value };
    update('input_args', args);
  };

  const addInputArg = () => update('input_args', [...(step.input_args || []), { key: '', value: '' }]);
  const removeInputArg = (ai) => update('input_args', (step.input_args || []).filter((_, i) => i !== ai));

  const updateObjPath = (pi, value) => {
    const paths = [...(step.obj_paths || [''])];
    paths[pi] = value;
    update('obj_paths', paths);
  };

  const addObjPath = () => update('obj_paths', [...(step.obj_paths || []), '']);
  const removeObjPath = (pi) => update('obj_paths', (step.obj_paths || []).filter((_, i) => i !== pi));

  const updateCondition = (field, value) => {
    update('condition', { ...(step.condition || {}), [field]: value });
  };

  // Determine on_error display: "abort", "continue", or "skip_to:STEP_ID"
  const isSkipTo = step.on_error?.startsWith('skip_to:');
  const errorPolicy = isSkipTo ? 'skip_to' : (step.on_error || 'abort');
  const skipToTarget = isSkipTo ? step.on_error.replace('skip_to:', '') : '';

  return (
    <Stack spacing={2.5}>
      {/* Row 1: name, type, move/delete buttons */}
      <Stack direction="row" spacing={1.5} alignItems="center">
        <TextField
          label="Step Name"
          value={step.name}
          onChange={(e) => update('name', e.target.value)}
          sx={{ flex: 1 }}
          id={`step-name-${index}`}
          autoComplete="off"
        />
        <FormControl sx={{ minWidth: 130 }}>
          <InputLabel>Type</InputLabel>
          <Select value={step.type} label="Type" onChange={(e) => update('type', e.target.value)}>
            {STEP_TYPES.map((t) => <MenuItem key={t} value={t}>{t}</MenuItem>)}
          </Select>
        </FormControl>
        <IconButton onClick={() => onMove(index, -1)} disabled={index === 0}>
          <SvgIcon fontSize="small"><ChevronUpIcon /></SvgIcon>
        </IconButton>
        <IconButton onClick={() => onMove(index, 1)} disabled={index === totalSteps - 1}>
          <SvgIcon fontSize="small"><ChevronDownIcon /></SvgIcon>
        </IconButton>
        <IconButton onClick={() => onRemove(index)} disabled={totalSteps === 1}>
          <SvgIcon fontSize="small"><TrashIcon /></SvgIcon>
        </IconButton>
      </Stack>

      {/* Type-specific fields */}
      {step.type === 'GET' && (
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">Parameter Paths</Typography>
          {(step.param_paths || ['']).map((p, pi) => (
            <Stack key={pi} direction="row" spacing={1.5} alignItems="center">
              <TextField
                fullWidth
                label={`Path ${pi + 1}`}
                placeholder="Device.DeviceInfo."
                value={p}
                onChange={(e) => updateParamPath(pi, e.target.value)}
                id={`step-${index}-path-${pi}`}
                autoComplete="off"
              />
              <IconButton onClick={() => removeParamPath(pi)} disabled={(step.param_paths || []).length <= 1}>
                <SvgIcon fontSize="small"><TrashIcon /></SvgIcon>
              </IconButton>
            </Stack>
          ))}
          <Button size="small" onClick={addParamPath}>Add Path</Button>
          <TextField
            label="Max Depth (0 = unlimited)"
            type="number"
            value={step.max_depth || 0}
            onChange={(e) => update('max_depth', parseInt(e.target.value) || 0)}
            sx={{ width: 220 }}
            id={`step-${index}-maxdepth`}
            autoComplete="off"
          />
        </Stack>
      )}

      {(step.type === 'SET' || step.type === 'ADD') && (
        <Stack spacing={2}>
          <TextField
            label="Object Path"
            fullWidth
            placeholder="Device.WiFi.SSID.1."
            value={step.obj_path || ''}
            onChange={(e) => update('obj_path', e.target.value)}
            id={`step-${index}-objpath`}
            autoComplete="off"
          />
          <Typography variant="body2" color="text.secondary">Parameters</Typography>
          {(step.param_settings || []).map((ps, pi) => (
            <Stack key={pi} direction="row" spacing={1.5} alignItems="center">
              <TextField
                label="Param"
                value={ps.param}
                onChange={(e) => updateParamSetting(pi, 'param', e.target.value)}
                sx={{ flex: 1 }}
                id={`step-${index}-ps-param-${pi}`}
                autoComplete="off"
              />
              <TextField
                label="Value"
                value={ps.value}
                onChange={(e) => updateParamSetting(pi, 'value', e.target.value)}
                sx={{ flex: 1 }}
                id={`step-${index}-ps-value-${pi}`}
                autoComplete="off"
              />
              <Tooltip title="Required">
                <Switch
                  checked={ps.required}
                  onChange={(e) => updateParamSetting(pi, 'required', e.target.checked)}
                />
              </Tooltip>
              <IconButton onClick={() => removeParamSetting(pi)}>
                <SvgIcon fontSize="small"><TrashIcon /></SvgIcon>
              </IconButton>
            </Stack>
          ))}
          <Button size="small" onClick={addParamSetting}>Add Parameter</Button>
        </Stack>
      )}

      {step.type === 'DELETE' && (
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">Object Paths to Delete</Typography>
          {(step.obj_paths || ['']).map((p, pi) => (
            <Stack key={pi} direction="row" spacing={1.5} alignItems="center">
              <TextField
                fullWidth
                label={`Path ${pi + 1}`}
                placeholder="Device.WiFi.SSID.2."
                value={p}
                onChange={(e) => updateObjPath(pi, e.target.value)}
                id={`step-${index}-delpath-${pi}`}
                autoComplete="off"
              />
              <IconButton onClick={() => removeObjPath(pi)} disabled={(step.obj_paths || []).length <= 1}>
                <SvgIcon fontSize="small"><TrashIcon /></SvgIcon>
              </IconButton>
            </Stack>
          ))}
          <Button size="small" onClick={addObjPath}>Add Path</Button>
        </Stack>
      )}

      {step.type === 'OPERATE' && (
        <Stack spacing={2}>
          <TextField
            label="Command"
            fullWidth
            placeholder="Device.Reboot()"
            value={step.command || ''}
            onChange={(e) => update('command', e.target.value)}
            id={`step-${index}-command`}
            autoComplete="off"
          />
          <TextField
            label="Command Key"
            fullWidth
            value={step.command_key || ''}
            onChange={(e) => update('command_key', e.target.value)}
            id={`step-${index}-cmdkey`}
            autoComplete="off"
          />
          <Typography variant="body2" color="text.secondary">Input Arguments</Typography>
          {(step.input_args || []).map((ia, ai) => (
            <Stack key={ai} direction="row" spacing={1.5} alignItems="center">
              <TextField
                label="Key"
                value={ia.key}
                onChange={(e) => updateInputArg(ai, 'key', e.target.value)}
                sx={{ flex: 1 }}
                id={`step-${index}-ia-key-${ai}`}
                autoComplete="off"
              />
              <TextField
                label="Value"
                value={ia.value}
                onChange={(e) => updateInputArg(ai, 'value', e.target.value)}
                sx={{ flex: 1 }}
                id={`step-${index}-ia-value-${ai}`}
                autoComplete="off"
              />
              <IconButton onClick={() => removeInputArg(ai)}>
                <SvgIcon fontSize="small"><TrashIcon /></SvgIcon>
              </IconButton>
            </Stack>
          ))}
          <Button size="small" onClick={addInputArg}>Add Argument</Button>
        </Stack>
      )}

      {step.type === 'CONDITION' && (
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">Condition</Typography>
          <Grid container spacing={2}>
            <Grid item xs={12} sm={3}>
              <TextField
                label="Source (saved result)"
                fullWidth
                value={step.condition?.source || ''}
                onChange={(e) => updateCondition('source', e.target.value)}
                id={`step-${index}-cond-source`}
                autoComplete="off"
              />
            </Grid>
            <Grid item xs={12} sm={3}>
              <TextField
                label="Path"
                fullWidth
                placeholder="Device.DeviceInfo."
                value={step.condition?.path || ''}
                onChange={(e) => updateCondition('path', e.target.value)}
                id={`step-${index}-cond-path`}
                autoComplete="off"
              />
            </Grid>
            <Grid item xs={12} sm={2}>
              <TextField
                label="Param"
                fullWidth
                placeholder="ModelName"
                value={step.condition?.param || ''}
                onChange={(e) => updateCondition('param', e.target.value)}
                id={`step-${index}-cond-param`}
                autoComplete="off"
              />
            </Grid>
            <Grid item xs={6} sm={2}>
              <FormControl fullWidth>
                <InputLabel>Operator</InputLabel>
                <Select
                  value={step.condition?.operator || '=='}
                  label="Operator"
                  onChange={(e) => updateCondition('operator', e.target.value)}
                >
                  {OPERATORS.map((op) => <MenuItem key={op} value={op}>{op}</MenuItem>)}
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={6} sm={2}>
              <TextField
                label="Value"
                fullWidth
                value={step.condition?.value || ''}
                onChange={(e) => updateCondition('value', e.target.value)}
                id={`step-${index}-cond-value`}
                autoComplete="off"
              />
            </Grid>
          </Grid>
          <Stack direction="row" spacing={2}>
            <FormControl sx={{ minWidth: 200 }}>
              <InputLabel>If True, jump to</InputLabel>
              <Select
                value={step.on_true || ''}
                label="If True, jump to"
                onChange={(e) => update('on_true', e.target.value)}
              >
                <MenuItem value="">Next step</MenuItem>
                {stepOptions.filter((s) => s.id !== step.id).map((s) => (
                  <MenuItem key={s.id} value={s.id}>{stepLabel(s)}</MenuItem>
                ))}
              </Select>
            </FormControl>
            <FormControl sx={{ minWidth: 200 }}>
              <InputLabel>If False, jump to</InputLabel>
              <Select
                value={step.on_false || ''}
                label="If False, jump to"
                onChange={(e) => update('on_false', e.target.value)}
              >
                <MenuItem value="">Next step</MenuItem>
                {stepOptions.filter((s) => s.id !== step.id).map((s) => (
                  <MenuItem key={s.id} value={s.id}>{stepLabel(s)}</MenuItem>
                ))}
              </Select>
            </FormControl>
          </Stack>
        </Stack>
      )}

      {step.type === 'DELAY' && (
        <TextField
          label="Duration (ms)"
          type="number"
          value={step.duration_ms || 1000}
          onChange={(e) => update('duration_ms', parseInt(e.target.value) || 0)}
          sx={{ width: 220 }}
          inputProps={{ min: 1, max: 60000 }}
          id={`step-${index}-delay`}
          autoComplete="off"
        />
      )}

      {/* Common fields for USP steps */}
      {!['CONDITION', 'DELAY'].includes(step.type) && (
        <Stack direction="row" spacing={2} alignItems="center">
          <TextField
            label="Save Result As"
            placeholder="variable_name"
            value={step.save_result_as || ''}
            onChange={(e) => update('save_result_as', e.target.value)}
            sx={{ width: 220 }}
            id={`step-${index}-saveas`}
            autoComplete="off"
          />
          <FormControl sx={{ minWidth: 150 }}>
            <InputLabel>On Error</InputLabel>
            <Select
              value={errorPolicy}
              label="On Error"
              onChange={(e) => {
                const val = e.target.value;
                if (val === 'skip_to') {
                  update('on_error', 'skip_to:');
                } else {
                  update('on_error', val);
                }
              }}
            >
              {ERROR_POLICIES.map((p) => <MenuItem key={p} value={p}>{p}</MenuItem>)}
              <MenuItem value="skip_to">skip_to</MenuItem>
            </Select>
          </FormControl>
          {errorPolicy === 'skip_to' && (
            <FormControl sx={{ minWidth: 180 }}>
              <InputLabel>Skip to step</InputLabel>
              <Select
                value={skipToTarget}
                label="Skip to step"
                onChange={(e) => update('on_error', 'skip_to:' + e.target.value)}
              >
                {stepOptions.filter((s) => s.id !== step.id).map((s) => (
                  <MenuItem key={s.id} value={s.id}>{stepLabel(s)}</MenuItem>
                ))}
              </Select>
            </FormControl>
          )}
        </Stack>
      )}
    </Stack>
  );
};
