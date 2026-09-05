import React from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import CiRulesetsList from './CiRulesetsList';
import CiRulesetEditor from './CiRulesetEditor';
import CiRulesetIntegration from './CiRulesetIntegration';

const CiRulesetsRoutes: React.FC = () => (
  <Routes>
    <Route index element={<CiRulesetsList />} />
    <Route path="new" element={<CiRulesetEditor />} />
    <Route path=":id/edit" element={<CiRulesetEditor />} />
    <Route path=":id/integration" element={<CiRulesetIntegration />} />
    <Route path="*" element={<Navigate to="/ci-rulesets" replace />} />
  </Routes>
);

export default CiRulesetsRoutes;
