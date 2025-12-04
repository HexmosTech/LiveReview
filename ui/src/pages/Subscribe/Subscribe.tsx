import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useOrgContext } from '../../hooks/useOrgContext';

// Declare Razorpay on window object
declare global {
  interface Window {
    Razorpay: any;
  }
}

interface IconProps {
  className?: string;
}

const CheckIcon: React.FC<IconProps> = ({ className }) => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
  >
    <path d="M5 12l5 5 9-9" />
  </svg>
);

const XIcon: React.FC<IconProps> = ({ className }) => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
  >
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
);

type BillingPeriod = 'monthly' | 'annual';

interface Feature {
  name: string;
  hobby: boolean | string;
  team: boolean | string;
  enterprise: boolean | string;
}

interface ContactModalProps {
  open: boolean;
  onClose: () => void;
}

const ContactSalesModal: React.FC<ContactModalProps> = ({ open, onClose }) => {
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [company, setCompany] = useState('');
  const [questions, setQuestions] = useState('');
  const [status, setStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle');
  const [error, setError] = useState<string | null>(null);
    const isLoading = status === 'loading';

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !isLoading) {
        event.preventDefault();
        onClose();
      }
    };

    if (open) {
      window.addEventListener('keydown', onKeyDown);
    }

    if (!open) {
      setName('');
      setEmail('');
      setCompany('');
      setQuestions('');
      setStatus('idle');
      setError(null);
    }
      return () => {
      window.removeEventListener('keydown', onKeyDown);
    };
    }, [open, onClose, isLoading]);

  if (!open) {
    return null;
  }

  const validate = () => {
    if (!name.trim() || !email.trim() || !company.trim()) {
      setError('Please fill in your name, work email, and company.');
      return false;
    }
    const emailRegex = /[^@\s]+@[^@\s]+\.[^@\s]+/;
    if (!emailRegex.test(email.trim())) {
      setError('Please enter a valid email address.');
      return false;
    }
    return true;
  };

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(null);
    if (!validate()) {
      return;
    }
    setStatus('loading');
    try {
      const embedsFields = [
        { name: 'Name', value: name, inline: true },
        { name: 'Email', value: email, inline: true },
        { name: 'Company', value: company, inline: false },
      ];

      if (questions.trim()) {
        embedsFields.push({ name: 'Questions', value: questions.trim(), inline: false });
      }

      const payload = {
        username: 'LiveReview Pricing',
        embeds: [
          {
            title: 'New Enterprise Enquiry',
            color: 0x8b5cf6,
            fields: embedsFields,
            footer: {
              text: `Submitted ${new Date().toLocaleString()}`,
            },
          },
        ],
      };

      const response = await fetch('https://discord.com/api/webhooks/1394676585151332402/Gwp-Qvt-_0UHK8yVZ_6rPxRHm3Y0x_cdQICstDD7MQ2eBNyqJaatL-uyixTnFMy8KV_H', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        throw new Error('Failed to send enquiry. Please try again.');
      }

      setStatus('success');
    } catch (submitError) {
      setStatus('error');
      setError(submitError instanceof Error ? submitError.message : 'Something went wrong. Please try again later.');
    }
  };

  return (
    <div 
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 backdrop-blur-sm px-4"
      onClick={(e) => {
        if (e.target === e.currentTarget && !isLoading) {
          onClose();
        }
      }}
    >
      <div className="relative w-full max-w-lg bg-slate-900 border border-slate-700 rounded-xl shadow-2xl">
        <button
          type="button"
          onClick={onClose}
          className="absolute top-4 right-4 text-slate-400 hover:text-white transition-colors"
          aria-label="Close dialog"
          disabled={isLoading}
        >
          <XIcon className="w-5 h-5" />
        </button>
        <div className="px-6 py-4 border-b border-slate-800 flex items-center justify-between">
          <h3 className="text-lg font-semibold text-white">Enterprise Enquiry</h3>
        </div>
        <form onSubmit={handleSubmit} className="px-6 py-6 space-y-5">
          <div>
            <label className="block text-sm font-medium text-slate-200 mb-2" htmlFor="enterprise-name">
              Name
            </label>
            <input
              id="enterprise-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full rounded-lg border border-slate-700 bg-slate-800 text-white px-3 py-2 focus:outline-none focus:ring-2 focus:ring-purple-500"
              placeholder="Your full name"
              disabled={isLoading}
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-slate-200 mb-2" htmlFor="enterprise-email">
              Work Email
            </label>
            <input
              id="enterprise-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full rounded-lg border border-slate-700 bg-slate-800 text-white px-3 py-2 focus:outline-none focus:ring-2 focus:ring-purple-500"
              placeholder="you@company.com"
              disabled={isLoading}
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-slate-200 mb-2" htmlFor="enterprise-company">
              Company
            </label>
            <input
              id="enterprise-company"
              type="text"
              value={company}
              onChange={(e) => setCompany(e.target.value)}
              className="w-full rounded-lg border border-slate-700 bg-slate-800 text-white px-3 py-2 focus:outline-none focus:ring-2 focus:ring-purple-500"
              placeholder="Company name"
              disabled={isLoading}
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-slate-200 mb-2" htmlFor="enterprise-questions">
              Questions (optional)
            </label>
            <textarea
              id="enterprise-questions"
              value={questions}
              onChange={(e) => setQuestions(e.target.value)}
              className="w-full min-h-[96px] rounded-lg border border-slate-700 bg-slate-800 text-white px-3 py-2 focus:outline-none focus:ring-2 focus:ring-purple-500 resize-y"
              placeholder="Anything specific you'd like help with?"
              disabled={isLoading}
            />
          </div>

          {error && <p className="text-sm text-rose-400">{error}</p>}

          {status === 'success' ? (
            <div className="bg-emerald-500/10 border border-emerald-400/40 rounded-lg px-4 py-3 text-sm text-emerald-300">
              Thanks! We received your enquiry. You’ll usually receive an email from Hexmos within 24 hours.
            </div>
          ) : (
            <button
              type="submit"
              className="w-full inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-md bg-blue-600 hover:bg-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-offset-2 focus:ring-offset-slate-900 text-white font-semibold shadow-md disabled:opacity-70 disabled:cursor-not-allowed disabled:bg-blue-800"
              disabled={isLoading}
            >
              {isLoading ? (
                <>
                  <span className="inline-flex h-4 w-4 animate-spin rounded-full border-2 border-white/60 border-t-transparent" />
                  Sending...
                </>
              ) : (
                'Submit enquiry'
              )}
            </button>
          )}
        </form>
      </div>
    </div>
  );
};

const Subscribe: React.FC = () => {
  const navigate = useNavigate();
  const { currentOrgId, currentOrg } = useOrgContext();
  const [billingPeriod, setBillingPeriod] = useState<BillingPeriod>('annual');
  const [contactOpen, setContactOpen] = useState(false);
  const [hoveredCell, setHoveredCell] = useState<{ feature: string; column: string } | null>(null);
  const [mousePosition, setMousePosition] = useState<{ x: number; y: number }>({ x: 0, y: 0 });

  const isAuthenticated = !!localStorage.getItem('accessToken');

  const features: Feature[] = [
    {
      name: 'Users Included',
      hobby: 'Single user',
      team: 'Unlimited',
      enterprise: 'Unlimited',
    },
    {
      name: 'Add Team Members',
      hobby: false,
      team: true,
      enterprise: true,
    },
    {
      name: 'Create New Orgs',
      hobby: false,
      team: true,
      enterprise: true,
    },
    {
      name: 'Organizations',
      hobby: 'Single org',
      team: 'Multiple orgs',
      enterprise: 'Multiple orgs',
    },
    {
      name: 'Daily Review Limit',
      hobby: '3 reviews/day',
      team: 'Unlimited',
      enterprise: 'Unlimited',
    },
    {
      name: 'AI Models',
      hobby: 'Cloud models only',
      team: 'Cloud models only',
      enterprise: 'Cloud + Self-hosted',
    },
    {
      name: 'Private Self-Hosted Deployment',
      hobby: false,
      team: false,
      enterprise: true,
    },
    {
      name: 'Custom Domain',
      hobby: false,
      team: false,
      enterprise: true,
    },
    {
      name: 'Custom Integrations',
      hobby: false,
      team: false,
      enterprise: true,
    },
    {
      name: 'Support',
      hobby: 'Email/GitHub',
      team: 'Prioritized Email/GitHub',
      enterprise: 'Dedicated Support + SLA',
    },
  ];

  const getTeamPrice = () => {
    if (billingPeriod === 'annual') {
      return {
        monthly: '$5',
        perUser: '$60/year',
        savings: '17% off',
      };
    }
    return {
      monthly: '$6',
      perUser: '$6/month',
      savings: null,
    };
  };

  const teamPrice = getTeamPrice();

  const handleGetTeamPlan = () => {
    if (!isAuthenticated) {
      // Redirect to sign in
      navigate('/signin', { state: { returnTo: '/subscribe' } });
      return;
    }
    
    // Navigate to checkout wizard with billing period
    navigate(`/checkout/team?period=${billingPeriod}`);
  };

  const renderFeatureValue = (value: boolean | string, featureName: string, column: string) => {
    const cellKey = `${featureName}-${column}`;
    const isHovered = hoveredCell?.feature === featureName && hoveredCell?.column === column;

    const handleMouseMove = (e: React.MouseEvent) => {
      setMousePosition({ x: e.clientX, y: e.clientY });
    };

    if (typeof value === 'string') {
      return (
        <div 
          className="relative group"
          onMouseEnter={() => setHoveredCell({ feature: featureName, column })}
          onMouseLeave={() => setHoveredCell(null)}
          onMouseMove={handleMouseMove}
        >
          <span className="text-sm text-slate-100 cursor-default">
            {value}
          </span>
          {isHovered && (
            <div 
              className="fixed z-10 px-3 py-2 bg-slate-950 text-white text-xs rounded-lg shadow-xl border border-slate-600 whitespace-nowrap pointer-events-none"
              style={{
                left: `${mousePosition.x + 12}px`,
                top: `${mousePosition.y - 12}px`,
              }}
            >
              {featureName}
            </div>
          )}
        </div>
      );
    }

    const IconComponent = value ? CheckIcon : XIcon;
    const iconColor = value ? 'text-emerald-400' : 'text-rose-400';
    const text = value ? 'Yes' : 'No';

    return (
      <div 
        className="relative group"
        onMouseEnter={() => setHoveredCell({ feature: featureName, column })}
        onMouseLeave={() => setHoveredCell(null)}
        onMouseMove={handleMouseMove}
      >
        <span className="inline-flex items-center gap-2 cursor-default">
          <IconComponent className={`w-4 h-4 ${iconColor}`} />
          <span className="text-sm text-slate-100">{text}</span>
        </span>
        {isHovered && (
          <div 
            className="fixed z-10 px-3 py-2 bg-slate-950 text-white text-xs rounded-lg shadow-xl border border-slate-600 whitespace-nowrap pointer-events-none"
            style={{
              left: `${mousePosition.x + 12}px`,
              top: `${mousePosition.y - 12}px`,
            }}
          >
            {featureName}
          </div>
        )}
      </div>
    );
  };

  return (
    <>
      <div className="min-h-screen bg-gradient-to-b from-slate-900 to-slate-800 py-12 px-4 sm:px-6 lg:px-8">
      <div className="max-w-7xl mx-auto">
        {/* Header */}
        <div className="text-center mb-12">
          <h1 className="text-4xl font-bold text-white mb-4">
            Choose Your Plan
          </h1>
          <p className="text-xl text-slate-300 mb-8">
            Start free, scale as you grow
          </p>

          {/* Billing Toggle */}
          <div className="inline-flex items-center bg-slate-800 rounded-lg p-1 border border-slate-700">
            <button
              onClick={() => setBillingPeriod('monthly')}
              className={`px-6 py-2 rounded-md text-sm font-medium transition-all ${
                billingPeriod === 'monthly'
                  ? 'bg-blue-600 text-white shadow-lg'
                  : 'text-slate-400 hover:text-white'
              }`}
            >
              Monthly
            </button>
            <button
              onClick={() => setBillingPeriod('annual')}
              className={`px-8 py-2 rounded-md text-sm font-medium transition-all ${
                billingPeriod === 'annual'
                  ? 'bg-blue-600 text-white shadow-lg'
                  : 'text-slate-400 hover:text-white'
              }`}
            >
              Annual <span className="ml-2 text-emerald-400 font-bold">17% off</span>
            </button>
          </div>
        </div>

        {/* Pricing Cards */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-8 mb-16">
          {/* Hobby Plan */}
          <div className="bg-slate-800 rounded-lg border border-slate-700 p-8 flex flex-col">
            <div className="mb-6">
              <h2 className="text-2xl font-bold text-white mb-2">Hobby</h2>
              <p className="text-slate-400 text-sm mb-4">
                Perfect for individual developers
              </p>
              <div className="flex items-baseline">
                <span className="text-4xl font-bold text-white">Free</span>
              </div>
            </div>

            <button
              type="button"
              onClick={() => navigate('/')}
              className="w-full bg-slate-700 hover:bg-slate-600 text-white font-semibold py-3 px-6 rounded-lg transition-colors mb-6"
            >
              Continue With Hobby
            </button>

            <div className="flex-grow">
              <h3 className="text-sm font-semibold text-slate-300 mb-4">
                Key Features:
              </h3>
              <ul className="space-y-3">
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Single user account
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Single organization
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    3 reviews per day
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Cloud AI models
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Email & GitHub support
                  </span>
                </li>
              </ul>
            </div>
          </div>

          {/* Team Plan */}
          <div className="bg-gradient-to-b from-blue-900/50 to-slate-800 rounded-lg border-2 border-blue-500 p-8 flex flex-col relative">
            <div className="absolute -top-4 left-1/2 -translate-x-1/2 bg-blue-600 text-white px-4 py-1 rounded-full text-sm font-semibold">
              Most Popular
            </div>

            <div className="mb-6">
              <h2 className="text-2xl font-bold text-white mb-2">Team</h2>
              <p className="text-slate-400 text-sm mb-4">
                For growing teams and organizations
              </p>
              
              <div className="flex items-baseline">
                <span className="text-4xl font-bold text-white">
                  {teamPrice.monthly}
                </span>
                <span className="text-slate-400 ml-2">
                  /user/month
                </span>
              </div>
              {billingPeriod === 'annual' && (
                <div className="mt-2">
                  <p className="text-sm text-blue-400">
                    {teamPrice.perUser}
                  </p>
                  <p className="text-sm text-emerald-400 font-semibold">
                    Save $12/user/year ({teamPrice.savings})
                  </p>
                </div>
              )}
            </div>

            <button
              type="button"
              onClick={handleGetTeamPlan}
              className="w-full bg-blue-600 hover:bg-blue-700 text-white font-semibold py-3 px-6 rounded-lg transition-colors mb-6 shadow-lg"
            >
              Get Team Plan
            </button>

            <div className="flex-grow">
              <h3 className="text-sm font-semibold text-slate-300 mb-4">
                Everything in Hobby, plus:
              </h3>
              <ul className="space-y-3">
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Unlimited team members
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Multiple organizations
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Unlimited reviews
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Cloud AI models
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Prioritized Github & Email support
                  </span>
                </li>
              </ul>
            </div>
          </div>

          {/* Enterprise Plan */}
          <div className="bg-slate-800 rounded-lg border border-slate-700 p-8 flex flex-col">
            <div className="mb-6">
              <h2 className="text-2xl font-bold text-white mb-2">Enterprise</h2>
              <p className="text-slate-400 text-sm mb-4">
                For large teams with advanced needs
              </p>
              <div className="flex items-baseline">
                <span className="text-2xl font-bold text-white">
                  Contact us
                </span>
              </div>
            </div>

            <button
              type="button"
              className="w-full bg-gray-600 font-semibold py-3 px-6 rounded-lg mb-6"
              onClick={() => setContactOpen(true)}
            >
              Contact Sales
            </button>

            <div className="flex-grow">
              <h3 className="text-sm font-semibold text-slate-300 mb-4">
                Everything in Team, plus:
              </h3>
              <ul className="space-y-3">
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Self-hosted deployment
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Self-hosted AI models
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Custom domain
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Full privacy & data control
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Custom integrations
                  </span>
                </li>
                <li className="flex items-start">
                  <CheckIcon className="w-5 h-5 text-emerald-400 mr-2 flex-shrink-0 mt-0.5" />
                  <span className="text-sm text-slate-200">
                    Dedicated support with SLA
                  </span>
                </li>
              </ul>
            </div>
          </div>
        </div>

        {/* Detailed Comparison Table */}
        <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
          <div className="px-6 py-4 bg-slate-900 border-b border-slate-700">
            <h2 className="text-2xl font-bold text-white">
              Detailed Feature Comparison
            </h2>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="bg-slate-900/50">
                  <th className="px-4 py-4 text-left text-sm font-semibold text-slate-300">
                    Feature
                  </th>
                  <th className="px-4 py-4 text-left text-sm font-semibold text-slate-300">
                    Hobby
                  </th>
                  <th className="px-4 py-4 text-left text-sm font-semibold text-blue-400">
                    Team
                  </th>
                  <th className="px-4 py-4 text-left text-sm font-semibold text-purple-400">
                    Enterprise
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-700">
                {features.map((feature, index) => (
                  <tr
                    key={index}
                    className="hover:bg-slate-700/30 transition-colors"
                  >
                    <td className="px-4 py-4 text-sm font-medium text-white">
                      {feature.name}
                    </td>
                    <td className="px-4 py-4">
                      {renderFeatureValue(feature.hobby, feature.name, 'hobby')}
                    </td>
                    <td className="px-4 py-4">
                      {renderFeatureValue(feature.team, feature.name, 'team')}
                    </td>
                    <td className="px-4 py-4">
                      {renderFeatureValue(feature.enterprise, feature.name, 'enterprise')}
                    </td>
                  </tr>
                ))}
                <tr className="bg-slate-900/50">
                  <td className="px-4 py-6"></td>
                  <td className="px-4 py-6">
                    <button
                      type="button"
                      onClick={() => navigate('/')}
                      className="w-full bg-slate-700 hover:bg-slate-600 text-white font-semibold py-2.5 px-4 rounded-lg transition-colors text-sm"
                    >
                      Continue With Hobby
                    </button>
                  </td>
                  <td className="px-4 py-6">
                    <button
                      type="button"
                      onClick={handleGetTeamPlan}
                      className="w-full bg-blue-600 hover:bg-blue-700 text-white font-semibold py-2.5 px-4 rounded-lg transition-colors shadow-lg text-sm"
                    >
                      Get Team Plan
                    </button>
                  </td>
                  <td className="px-4 py-6">
                    <button
                      type="button"
                      className="w-full bg-gray-600 hover:bg-gray-500 text-white font-semibold py-2.5 px-4 rounded-lg transition-colors text-sm"
                      onClick={() => setContactOpen(true)}
                    >
                      Contact Sales
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        {/* FAQ Section */}
        <div className="mt-16 text-center">
          <h2 className="text-2xl font-bold text-white mb-4">
            Have questions?
          </h2>
          <p className="text-slate-300 mb-6">
            Check out our{' '}
            <a
              href="https://github.com/HexmosTech/LiveReview/wiki"
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-400 hover:text-blue-300 underline"
            >
              documentation
            </a>{' '}
            or{' '}
            <a
              href="https://github.com/HexmosTech/LiveReview/discussions"
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-400 hover:text-blue-300 underline"
            >
              contact us
            </a>
          </p>
        </div>
        </div>
      </div>
      <ContactSalesModal open={contactOpen} onClose={() => setContactOpen(false)} />
    </>
  );
};

export default Subscribe;
