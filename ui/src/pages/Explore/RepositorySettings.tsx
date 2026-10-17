import React, { useEffect, useState } from 'react';
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom';
import { LuDatabase } from 'react-icons/lu';
import { Button, Card, EmptyState, Icons, PageHeader, Spinner } from '../../components/UIPrimitives';
import RepoCacheSection from '../../components/Explore/RepoCacheSection';
import RepoAutoApproveSection from '../../components/Explore/RepoAutoApproveSection';
import { getRepository } from '../../api/repositories';
import { Repository } from '../../types/explore';

// Per-repository settings, laid out like the main Settings page: sections on the left, the selected one on the right.
const SECTIONS = [
  { id: 'repo-cache', name: 'Repo cache', icon: <LuDatabase className="w-4 h-4" /> },
  { id: 'auto-approve', name: 'Auto Approve', icon: <Icons.Success /> },
];

const RepositorySettings: React.FC = () => {
  const { repoId } = useParams<{ repoId: string }>();
  const location = useLocation();
  const navigate = useNavigate();
  const [repo, setRepo] = useState<Repository | null>(null);
  const [error, setError] = useState(false);
  const requested = location.hash.substring(1);
  const active = SECTIONS.some((s) => s.id === requested) ? requested : SECTIONS[0].id;

  useEffect(() => {
    setError(false);
    getRepository(Number(repoId)).then(setRepo).catch(() => setError(true));
  }, [repoId]);

  const back = (
    <Button as={Link} to="/explore/repositories" variant="ghost" icon={<Icons.Back />} className="mb-4 -ml-4 text-sm">
      Back to Repositories
    </Button>
  );

  if (error) {
    return (
      <div className="container mx-auto px-4 py-8">
        {back}
        <EmptyState icon={<Icons.Error />} title="Repository not found" description="It may have been removed, or it belongs to another organization." />
      </div>
    );
  }
  if (!repo) {
    return <div className="flex justify-center py-16"><Spinner /></div>;
  }

  return (
    <div className="container mx-auto px-4 py-8">
      {back}
      <PageHeader title={repo.full_name} description="Repository settings" />
      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        <div className="md:col-span-1">
          <Card>
            <div className="space-y-2">
              {SECTIONS.map((s) => (
                <Button
                  key={s.id}
                  variant={active === s.id ? 'primary' : 'ghost'}
                  fullWidth
                  className="!justify-start"
                  icon={s.icon}
                  onClick={() => navigate(`#${s.id}`)}
                >
                  {s.name}
                </Button>
              ))}
            </div>
          </Card>
        </div>
        <div className="md:col-span-3">
          <Card>
            {active === 'repo-cache' && <RepoCacheSection repo={repo} />}
            {active === 'auto-approve' && <RepoAutoApproveSection repositoryId={repo.id} />}
          </Card>
        </div>
      </div>
    </div>
  );
};

export default RepositorySettings;
